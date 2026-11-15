package anomaly

// Engine scores persisted chunks off the capture path. Input is bounded by
// bytes and dropped when full; capture and the writer never wait for it.
import (
	"container/list"
	"context"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"gomate/internal/analyze"
	"gomate/internal/model"
)

const (
	reportBits   = 6.0 // stream score that is stored and shown
	maxStreams   = 8192
	maxReasons   = 10
	queueBytes   = 32 << 20
	flushPeriod  = time.Second
	pendingLimit = 4096
)

type Sink interface {
	SaveAnomalies(context.Context, []Result) error
}

type Result struct {
	StreamID  int64    `json:"streamId"`
	Service   int      `json:"service"`
	Score     float64  `json:"score"`
	Kind      string   `json:"kind"`
	Summary   string   `json:"summary"`
	Reasons   []Reason `json:"reasons"`
	UpdatedMs int64    `json:"updatedMs"`
}

type ServiceStats struct {
	Port      int    `json:"port"`
	Messages  uint64 `json:"messages"`
	Endpoints int    `json:"endpoints"`
	Warm      bool   `json:"warm"`
	Reported  int64  `json:"reported"`
}

type Stats struct {
	Services   []ServiceStats `json:"services"`
	Drops      int64          `json:"drops"`
	QueueBytes int64          `json:"queueBytes"`
	WarmAfter  int            `json:"warmAfter"`
}

type pendingRequest struct{ tpl, summary string }

type streamState struct {
	key      string
	id       int64
	service  int
	pending  []pendingRequest
	h2       map[string]pendingRequest
	best     [2]float64 // the two most unusual messages, usually request and response
	reasons  []Reason
	summary  string
	reported float64
}

type feedback struct {
	service int
	reasons []Reason
}

type Engine struct {
	catalog *analyze.Catalog
	sink    Sink
	onSaved func([]Result)
	timeout time.Duration
	queue   chan []*model.Chunk
	accept  chan feedback
	done    chan struct{}
	closed  sync.Once
	queued  atomic.Int64
	drops   atomic.Int64
	stats   atomic.Pointer[Stats]

	services map[int]*serviceModel
	streams  map[string]*list.Element
	lru      *list.List
	pending  map[int64]Result
}

func New(c *analyze.Catalog, sink Sink, timeout time.Duration, onSaved ...func([]Result)) *Engine {
	e := newEngine(c, sink, timeout)
	if len(onSaved) > 0 {
		e.onSaved = onSaved[0]
	}
	go e.run()
	return e
}

func newEngine(c *analyze.Catalog, sink Sink, timeout time.Duration) *Engine {
	e := &Engine{catalog: c, sink: sink, timeout: timeout, queue: make(chan []*model.Chunk, 1024), accept: make(chan feedback, 64), done: make(chan struct{}), services: map[int]*serviceModel{}, streams: map[string]*list.Element{}, lru: list.New(), pending: map[int64]Result{}}
	e.publish()
	return e
}

// Submit hands over committed chunks. Chunks are read-only from here on.
func (e *Engine) Submit(chunks []*model.Chunk) {
	var size int64
	batch := make([]*model.Chunk, 0, len(chunks))
	for _, c := range chunks {
		if c.Stream.ID != 0 && len(c.Packets) > 0 {
			batch = append(batch, c)
			size += c.Bytes + 1024
		}
	}
	if len(batch) == 0 {
		return
	}
	if e.queued.Add(size) > queueBytes {
		e.queued.Add(-size)
		e.drops.Add(int64(len(batch)))
		return
	}
	select {
	case e.queue <- batch:
	default:
		e.queued.Add(-size)
		e.drops.Add(int64(len(batch)))
	}
}

func (e *Engine) Accept(service int, reasons []Reason) {
	select {
	case e.accept <- feedback{service, reasons}:
	default:
	}
}

func (e *Engine) Stats() Stats { return *e.stats.Load() }

func (e *Engine) Close() {
	e.closed.Do(func() { close(e.queue) })
	<-e.done
}

func (e *Engine) run() {
	defer close(e.done)
	tick := time.NewTicker(flushPeriod)
	defer tick.Stop()
	for {
		select {
		case batch, ok := <-e.queue:
			if !ok {
				e.flush()
				return
			}
			for _, c := range batch {
				e.process(c)
				e.queued.Add(-(c.Bytes + 1024))
			}
		case f := <-e.accept:
			if s := e.services[f.service]; s != nil {
				for _, r := range f.reasons {
					s.accept(r)
				}
			}
		case <-tick.C:
			e.flush()
			e.publish()
		}
	}
}

func (e *Engine) flush() {
	if len(e.pending) == 0 || e.sink == nil {
		return
	}
	out := make([]Result, 0, len(e.pending))
	for _, r := range e.pending {
		out = append(out, r)
	}
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	err := e.sink.SaveAnomalies(ctx, out)
	cancel()
	if err != nil {
		slog.Warn("anomaly results not saved; will retry", "error", err, "count", len(out))
		if len(e.pending) > pendingLimit {
			clear(e.pending)
		}
		return
	}
	if e.onSaved != nil {
		e.onSaved(out)
	}
	clear(e.pending)
}

func (e *Engine) publish() {
	st := &Stats{Services: []ServiceStats{}, Drops: e.drops.Load(), QueueBytes: e.queued.Load(), WarmAfter: warmService}
	for port, s := range e.services {
		x := s.stats
		x.Port = port
		x.Messages = s.n
		x.Endpoints = len(s.templates)
		x.Warm = s.n >= warmService
		st.Services = append(st.Services, x)
	}
	sort.Slice(st.Services, func(i, j int) bool { return st.Services[i].Port < st.Services[j].Port })
	e.stats.Store(st)
}

func (e *Engine) stream(c *model.Chunk) *streamState {
	if el := e.streams[c.Stream.Key]; el != nil {
		e.lru.MoveToBack(el)
		return el.Value.(*streamState)
	}
	st := &streamState{key: c.Stream.Key, id: c.Stream.ID, service: c.Stream.Service, h2: map[string]pendingRequest{}}
	e.streams[st.key] = e.lru.PushBack(st)
	for e.lru.Len() > maxStreams {
		old := e.lru.Remove(e.lru.Front()).(*streamState)
		delete(e.streams, old.key)
	}
	return st
}

func (e *Engine) process(c *model.Chunk) {
	cfg, _ := e.catalog.Service(c.Stream.Service)
	s := e.services[c.Stream.Service]
	if s == nil {
		s = newService()
		e.services[c.Stream.Service] = s
	}
	st := e.stream(c)
	decoded := cfg.HTTP || cfg.WebSockets || cfg.URLDecode
	for _, p := range c.Packets {
		var m message
		switch {
		case p.Kind == "http":
			m = parseHTTP1(p)
			if !m.incoming && m.interim {
				continue
			}
		case p.Kind == "http2-headers":
			m = parseHTTP2Headers(p)
		case p.Kind == "http2-body" || p.Kind == "grpc":
			m = message{incoming: p.Incoming, h2: p.Metadata["streamId"]}
			addBody(&m, p.Metadata["contentType"], p.Content)
			if p.Kind == "grpc" && p.Incoming {
				m.fields = append(m.fields[:0], field{key: "grpc.message", value: truncate(p.Content), size: len(p.Content), kind: kindValue})
			}
		case p.Kind == "websocket":
			if v, ok := parseJSON(p.Content); ok {
				m = message{incoming: p.Incoming, tpl: "WS", summary: "WS " + preview(p.Content, 120)}
				kind := kindValue
				if !p.Incoming {
					kind = kindPresence
				}
				addJSON(&m, "json", v, kind, linkWeak, 0)
				if !p.Incoming {
					m.tpl = "WS ⇐"
				}
			} else {
				m = parseText(p, "WS", s.verb)
			}
		case p.Kind == "" && !decoded:
			m = parseText(p, c.Stream.Protocol, s.verb)
		default:
			continue
		}
		e.pair(s, st, &m, p)
		e.handle(s, st, &m)
	}
	if c.Closed {
		if el := e.streams[st.key]; el != nil {
			e.lru.Remove(el)
			delete(e.streams, st.key)
		}
	}
}

func truncate(b []byte) []byte {
	if len(b) > valueLimit {
		return b[:valueLimit]
	}
	return b
}

// pair gives responses the template of their request.
func (e *Engine) pair(s *serviceModel, st *streamState, m *message, p model.Packet) {
	switch {
	case m.incoming && m.path != "":
		s.templatize(m)
		if m.h2 != "" {
			st.h2[m.h2] = pendingRequest{m.tpl, m.summary}
		} else if len(st.pending) < 64 {
			st.pending = append(st.pending, pendingRequest{m.tpl, m.summary})
		}
	case m.tpl != "":
	case m.h2 != "":
		req, ok := st.h2[m.h2]
		if !ok {
			req = pendingRequest{"H2 " + p.Metadata["path"], "H2 " + p.Metadata["path"]}
		}
		m.summary = req.summary
		switch {
		case m.incoming:
			m.tpl = req.tpl + " · тело"
		case m.status != "":
			m.tpl = req.tpl + " ⇐ ответ"
		default:
			m.tpl = req.tpl + " ⇐ тело"
		}
	case !m.incoming:
		req := pendingRequest{"?", "ответ без запроса"}
		if len(st.pending) > 0 {
			req = st.pending[0]
			st.pending = st.pending[1:]
		}
		m.tpl = req.tpl + " ⇐ ответ"
		m.summary = req.summary
	default:
		m.tpl = "?"
	}
	if m.summary == "" {
		m.summary = m.tpl
	}
}

func (e *Engine) handle(s *serviceModel, st *streamState, m *message) {
	reasons := s.score(m)
	s.learn(m)
	changed := false
	total := 0.0
	for _, r := range reasons {
		total += clampReason(r.Bits)
	}
	if total > st.best[1] {
		if total > st.best[0] {
			st.best[0], st.best[1] = total, st.best[0]
			if m.incoming || st.summary == "" {
				st.summary = m.summary
			}
		} else {
			st.best[1] = total
		}
		st.reasons = mergeReasons(st.reasons, reasons)
		changed = true
	}
	if st.summary == "" && m.incoming {
		st.summary = m.summary
	}
	if changed {
		e.report(s, st)
	}
}

// mergeReasons keeps the strongest reason per detector, field and template.
func mergeReasons(old, add []Reason) []Reason {
	out := append([]Reason{}, old...)
	for _, r := range add {
		replaced := false
		for i, x := range out {
			if x.Detector == r.Detector && x.Field == r.Field && x.Template == r.Template {
				if r.Bits > x.Bits {
					out[i] = r
				}
				replaced = true
				break
			}
		}
		if !replaced {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Bits > out[j].Bits })
	if len(out) > 2*maxReasons {
		out = out[:2*maxReasons]
	}
	return out
}

func clampReason(b float64) float64 {
	if b > reasonCap {
		return reasonCap
	}
	return b
}

func (e *Engine) report(s *serviceModel, st *streamState) {
	score := st.best[0] + st.best[1]
	all := append([]Reason{}, st.reasons...)
	if score < reportBits || score <= st.reported {
		return
	}
	if st.reported < reportBits {
		s.stats.Reported++
	}
	st.reported = score
	sort.SliceStable(all, func(i, j int) bool { return all[i].Bits > all[j].Bits })
	if len(all) > maxReasons {
		all = all[:maxReasons]
	}
	e.pending[st.id] = Result{StreamID: st.id, Service: st.service, Score: capBits(score, 1000), Summary: st.summary, Reasons: all, UpdatedMs: time.Now().UnixMilli()}
	if len(e.pending) > pendingLimit {
		e.flush()
	}
}
