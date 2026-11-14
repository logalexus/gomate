package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"gomate/internal/analyze"
	"gomate/internal/config"
	"gomate/internal/metrics"
	"gomate/internal/model"
)

type Sink interface {
	SaveChunks(context.Context, []*model.Chunk) error
}
type Writer struct {
	cfg     config.Config
	metrics *metrics.Metrics
	catalog *analyze.Catalog
	sink    Sink
	notify  func()
	queue   chan *model.Chunk
	wg      sync.WaitGroup
	ctx     context.Context
	lossMu  sync.Mutex
	losses  map[string]*model.Chunk
	observe atomic.Pointer[func([]*model.Chunk)]
}

func New(ctx context.Context, cfg config.Config, m *metrics.Metrics, c *analyze.Catalog, s Sink, notify func()) *Writer {
	w := &Writer{cfg: cfg, metrics: m, catalog: c, sink: s, notify: notify, queue: make(chan *model.Chunk, 4096), ctx: ctx, losses: map[string]*model.Chunk{}}
	w.wg.Add(1)
	go w.run()
	return w
}
func (w *Writer) Submit(c *model.Chunk) bool {
	cost := c.Bytes + int64(len(c.Packets)*256) + 1024
	for !metrics.Reserve(&w.metrics.WriteBytes, cost, w.cfg.WriteBytes) {
		if w.cfg.PCAP == "" || cost > w.cfg.WriteBytes || w.ctx.Err() != nil {
			w.metrics.WriteDrops.Add(1)
			w.recordLoss(c)
			return false
		}
		select {
		case <-w.ctx.Done():
			return false
		case <-time.After(time.Millisecond):
		}
	}
	c.QueueCost = cost
	c.Enqueued = time.Now()
	if w.cfg.PCAP != "" {
		select {
		case w.queue <- c:
			return true
		case <-w.ctx.Done():
			w.metrics.WriteBytes.Add(-cost)
			w.metrics.WriteDrops.Add(1)
			w.recordLoss(c)
			return false
		}
	}
	select {
	case w.queue <- c:
		return true
	default:
		w.metrics.WriteBytes.Add(-cost)
		w.metrics.WriteDrops.Add(1)
		w.recordLoss(c)
		return false
	}
}

// Observe receives every committed batch after the commit. It must not block.
func (w *Writer) Observe(f func([]*model.Chunk)) { w.observe.Store(&f) }

func (w *Writer) Close() error {
	close(w.queue)
	w.wg.Wait()
	if w.cfg.PCAP != "" && w.metrics.WriteDrops.Load() > 0 {
		return fmt.Errorf("PCAP output incomplete: %d chunks lost", w.metrics.WriteDrops.Load())
	}
	return nil
}
func (w *Writer) run() {
	defer w.wg.Done()
	repair := time.NewTicker(time.Second)
	defer repair.Stop()
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()
	var timerC <-chan time.Time
	batch := make([]*model.Chunk, 0, 256)
	var size int64
	processor := analyze.NewProcessor(w.catalog)
	flush := func() {
		losses := w.snapshotLosses()
		if len(batch) == 0 && len(losses) == 0 {
			return
		}
		timer.Stop()
		timerC = nil
		for _, c := range batch {
			processor.Process(c)
			w.metrics.AnalysisBytes.Store(int64(processor.Size()))
			w.metrics.AnalysisEvictions.Store(processor.Evictions)
			w.metrics.DecodeErrors.Store(processor.DecodeErrors)
		}
		persist := make([]*model.Chunk, 0, len(batch)+len(losses))
		persist = append(persist, batch...)
		persist = append(persist, losses...)
		var err error
		for attempt := 0; attempt < 3; attempt++ {
			ctx, cancel := context.WithTimeout(w.ctx, w.cfg.DBTimeout)
			err = w.sink.SaveChunks(ctx, persist)
			cancel()
			if err == nil {
				break
			}
			w.metrics.DBErrors.Add(1)
			slog.Warn("batch write failed", "attempt", attempt+1, "error", err)
			select {
			case <-w.ctx.Done():
			case <-time.After(time.Duration(attempt+1) * 100 * time.Millisecond):
			}
		}
		for _, c := range batch {
			w.metrics.WriteBytes.Add(-c.QueueCost)
			if err == nil {
				w.metrics.PersistedChunks.Add(1)
				w.metrics.PersistedBytes.Add(c.Bytes)
			} else {
				w.metrics.WriteDrops.Add(1)
				w.recordLoss(c)
				processor.Forget(c.Stream.Key)
			}
		}
		if err == nil {
			if f := w.observe.Load(); f != nil {
				(*f)(batch)
			}
			w.ackLosses(losses)
			w.metrics.LastCommit.Store(time.Now().Unix())
			w.notify()
		}
		clear(batch)
		batch = batch[:0]
		size = 0
		w.metrics.PendingSince.Store(0)
	}
	for {
		select {
		case c, ok := <-w.queue:
			if !ok {
				flush()
				return
			}
			if len(batch) == 0 {
				w.metrics.PendingSince.Store(c.Enqueued.Unix())
				timer.Reset(100 * time.Millisecond)
				timerC = timer.C
			}
			batch = append(batch, c)
			size += c.Bytes
			if len(batch) >= 256 || size >= 1<<20 {
				flush()
			}
		case <-timerC:
			flush()
		case <-repair.C:
			if len(batch) == 0 {
				flush()
			}
		}
	}
}

// Preserve bounded loss metadata independently of the saturated payload queue.
// A later successful transaction marks even previously stored rows incomplete.
func (w *Writer) recordLoss(c *model.Chunk) {
	w.lossMu.Lock()
	defer w.lossMu.Unlock()
	if len(w.losses) >= 4096 && w.losses[c.Stream.Key] == nil {
		for key := range w.losses {
			delete(w.losses, key)
			break
		}
	}
	v := *c
	v.Token = c.Token + "/loss"
	v.Stream.Incomplete = true
	v.Stream.Patterns = nil
	v.Packets = nil
	v.Bytes = 0
	v.QueueCost = 0
	w.losses[c.Stream.Key] = &v
}
func (w *Writer) snapshotLosses() []*model.Chunk {
	w.lossMu.Lock()
	defer w.lossMu.Unlock()
	out := make([]*model.Chunk, 0, len(w.losses))
	for _, v := range w.losses {
		out = append(out, v)
	}
	return out
}
func (w *Writer) ackLosses(losses []*model.Chunk) {
	w.lossMu.Lock()
	defer w.lossMu.Unlock()
	for _, v := range losses {
		if w.losses[v.Stream.Key] == v {
			delete(w.losses, v.Stream.Key)
		}
	}
}
