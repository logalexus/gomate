package anomaly

// Engine scores persisted chunks off the capture path. Input is bounded by
// bytes and dropped when full; capture and the writer never wait for it.
import (
	"container/list"
	"context"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"gomate/internal/analyze"
	"gomate/internal/model"
)

const (
	reportBits   = 6.0  // stream score that is stored and shown
	theftBits    = 40.0 // a flag leaving through an unrelated session
	maxStreams   = 8192
	maxFlags     = 50000
	maxOrphans   = 20000
	maxLinkVals  = 64
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
	Port       int    `json:"port"`
	Messages   uint64 `json:"messages"`
	Endpoints  int    `json:"endpoints"`
	Warm       bool   `json:"warm"`
	FlagsPut   int64  `json:"flagsPut"`
	FlagsGot   int64  `json:"flagsChecked"`
	FlagsStole int64  `json:"flagsStolen"`
	FlagsOther int64  `json:"flagsUnknown"`
	Reported   int64  `json:"reported"`
}

type Stats struct {
	Services    []ServiceStats `json:"services"`
	Drops       int64          `json:"drops"`
	QueueBytes  int64          `json:"queueBytes"`
	FlagPattern bool           `json:"flagPattern"`
	WarmAfter   int            `json:"warmAfter"`
}

type values struct{ weak, strong map[uint64]bool }

type pendingRequest struct{ tpl, summary string }

type theft struct {
	flag string
	put  int64
	weak int
}

type streamState struct {
	key      string
	id       int64
	service  int
	pending  []pendingRequest
	h2       map[string]pendingRequest
	vals     *values
	best     [2]float64 // the two most unusual messages, usually request and response
	reasons  []Reason
	stolen   []theft
	flagSeen map[string]bool
	summary  string
	reported float64
}

type flagInfo struct {
	key  string
	id   int64
	vals *values
}

// orphan is a flag that left the service before its placement was processed.
type orphan struct {
	flag string
	st   *streamState
}

type feedback struct {
	service int
	reasons []Reason
}

type Engine struct {
	catalog *analyze.Catalog
	sink    Sink
	timeout time.Duration
	queue   chan []*model.Chunk
	accept  chan feedback
	done    chan struct{}
	closed  sync.Once
	queued  atomic.Int64
	drops   atomic.Int64
	stats   atomic.Pointer[Stats]

	services  map[int]*serviceModel
	streams   map[string]*list.Element
	lru       *list.List
	flags     map[string]*flagInfo
	flagOrder []string
	orphans   map[string][]*streamState
	orphanLog []orphan
	pending   map[int64]Result
	flagRe    []*regexp.Regexp
}

func New(c *analyze.Catalog, sink Sink, timeout time.Duration) *Engine {
	e := newEngine(c, sink, timeout)
	go e.run()
	return e
}

func newEngine(c *analyze.Catalog, sink Sink, timeout time.Duration) *Engine {
	e := &Engine{catalog: c, sink: sink, timeout: timeout, queue: make(chan []*model.Chunk, 1024), accept: make(chan feedback, 64), done: make(chan struct{}), services: map[int]*serviceModel{}, streams: map[string]*list.Element{}, lru: list.New(), flags: map[string]*flagInfo{}, orphans: map[string][]*streamState{}, pending: map[int64]Result{}}
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
	clear(e.pending)
}

func (e *Engine) publish() {
	st := &Stats{Services: []ServiceStats{}, Drops: e.drops.Load(), QueueBytes: e.queued.Load(), FlagPattern: len(e.catalog.FlagMatchers()) > 0, WarmAfter: warmService}
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
	st := &streamState{key: c.Stream.Key, id: c.Stream.ID, service: c.Stream.Service, h2: map[string]pendingRequest{}, vals: &values{weak: map[uint64]bool{}, strong: map[uint64]bool{}}, flagSeen: map[string]bool{}}
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
	e.flagRe = e.catalog.FlagMatchers()
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
		e.handle(s, st, &m, p.Content)
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

func (e *Engine) handle(s *serviceModel, st *streamState, m *message, content []byte) {
	reasons := s.score(m)
	s.learn(m)
	if m.incoming {
		for _, f := range m.fields {
			if f.link == linkNone || f.size < 3 {
				continue
			}
			set := st.vals.weak
			if f.link == linkStrong {
				set = st.vals.strong
			}
			if len(set) < maxLinkVals {
				set[hash64(f.value)] = true
			}
		}
	} else {
		for _, f := range m.fields {
			if f.link == linkStrong && f.size >= 3 && len(st.vals.strong) < maxLinkVals {
				st.vals.strong[hash64(f.value)] = true
			}
		}
	}
	changed := e.checkFlags(s, st, m.incoming, content)
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
	kind := ""
	if n := len(st.stolen); n > 0 {
		kind = "theft"
		bits := theftBits + 2*math.Log2(float64(n))
		score += bits
		first := st.stolen[0]
		text := fmt.Sprintf("флаг %s положен в сессии #%d, а отдан этой сессии; общих значений с ней: %d (для своей сессии нужно ≥2 или общий cookie/токен)", first.flag, first.put, first.weak)
		if n > 1 {
			text = fmt.Sprintf("отдано %d чужих флагов, например %s из сессии #%d; общих значений с сессиями закладки нет", n, first.flag, first.put)
		}
		all = append(all, Reason{Bits: capBits(bits, 1000), Detector: "theft", Text: text})
	}
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
	e.pending[st.id] = Result{StreamID: st.id, Service: st.service, Score: capBits(score, 1000), Kind: kind, Summary: st.summary, Reasons: all, UpdatedMs: time.Now().UnixMilli()}
	if len(e.pending) > pendingLimit {
		e.flush()
	}
}

// checkFlags tracks flag provenance without IP addresses: a flag entering the
// service marks a placing session; a flag leaving through a session that shares
// no token and fewer than two request values with it is reported as stolen.
// A flag leaving before its placement was processed waits as an orphan.
func (e *Engine) checkFlags(s *serviceModel, st *streamState, incoming bool, content []byte) bool {
	changed := false
	for _, re := range e.flagRe {
		for _, loc := range re.FindAllIndex(content, 64) {
			flag := string(content[loc[0]:loc[1]])
			if st.flagSeen[flag] {
				continue
			}
			st.flagSeen[flag] = true
			if !incoming {
				if info := e.flags[flag]; info != nil {
					changed = e.resolve(s, st, flag, info) || changed
				} else {
					e.addOrphan(s, st, flag)
				}
				continue
			}
			if _, ok := e.flags[flag]; ok {
				continue
			}
			if len(e.flagOrder) >= maxFlags {
				delete(e.flags, e.flagOrder[0])
				e.flagOrder = e.flagOrder[1:]
			}
			info := &flagInfo{key: st.key, id: st.id, vals: st.vals}
			e.flags[flag] = info
			e.flagOrder = append(e.flagOrder, flag)
			s.stats.FlagsPut++
			for _, o := range e.orphans[flag] {
				s.stats.FlagsOther--
				if e.resolve(s, o, flag, info) {
					e.report(s, o)
				}
			}
			delete(e.orphans, flag)
		}
	}
	return changed
}

func (e *Engine) addOrphan(s *serviceModel, st *streamState, flag string) {
	if len(e.orphanLog) >= maxOrphans {
		old := e.orphanLog[0]
		e.orphanLog = e.orphanLog[1:]
		if list := e.orphans[old.flag]; len(list) > 0 {
			e.orphans[old.flag] = list[1:]
			if len(list) == 1 {
				delete(e.orphans, old.flag)
			}
		}
	}
	e.orphans[flag] = append(e.orphans[flag], st)
	e.orphanLog = append(e.orphanLog, orphan{flag, st})
	s.stats.FlagsOther++
}

func (e *Engine) resolve(s *serviceModel, st *streamState, flag string, info *flagInfo) bool {
	if info.key == st.key {
		return false
	}
	weak, strong := shared(st.vals.weak, info.vals.weak), shared(st.vals.strong, info.vals.strong)
	if strong > 0 || weak >= 2 {
		s.stats.FlagsGot++
		return false
	}
	s.stats.FlagsStole++
	st.stolen = append(st.stolen, theft{flag: preview([]byte(flag), 48), put: info.id, weak: weak})
	return true
}

func shared(a, b map[uint64]bool) int {
	n := 0
	for k := range a {
		if b[k] {
			n++
		}
	}
	return n
}
