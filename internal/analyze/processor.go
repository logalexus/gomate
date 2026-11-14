package analyze

import (
	"container/list"
	"gomate/internal/model"
	"strings"
	"time"
)

// TailBytes is the explicit regex look-behind window. Literal patterns are
// limited to 4096 bytes, so all literal boundary crossings fit in this window.
const TailBytes = 64 << 10

// MatchState is serializable so lookback can atomically checkpoint search state.
type MatchState struct {
	Tails [2][]byte `json:"tails"`
}

func (c *Catalog) AnalyzeStreaming(chunk *model.Chunk, state *MatchState) {
	snap := c.value.Load()
	textPatterns := map[int]bool{}
	for _, x := range snap.patterns {
		textPatterns[x.p.ID] = x.p.SearchType != "SUBBYTES"
	}
	ids := map[int]bool{}
	chunk.Ignore = false
	for i := range chunk.Packets {
		p := &chunk.Packets[i]
		if p.Kind == "decode-error" {
			continue
		}
		idx := 0
		if p.Incoming {
			idx = 1
		}
		prefix := []byte(nil)
		if p.Kind == "" && chunk.Stream.Protocol == "TCP" {
			if p.Incomplete {
				state.Tails[idx] = nil
			}
			prefix = state.Tails[idx]
		}
		b := make([]byte, 0, len(prefix)+len(p.Content))
		b = append(b, prefix...)
		b = append(b, p.Content...)
		one := model.Chunk{Stream: chunk.Stream, Packets: []model.Packet{{Content: b, Incoming: p.Incoming}}}
		analyzeSnapshot(&one, snap)
		if one.Ignore {
			chunk.Ignore = true
		}
		p.Matches = []model.Match{}
		for _, m := range one.Packets[0].Matches {
			offset := len(prefix)
			if textPatterns[m.PatternID] {
				offset = utf16lenValid(prefix)
			}
			if m.End < offset {
				continue
			}
			m.Start -= offset
			m.End -= offset
			if m.Start < 0 {
				m.CrossBoundary = true
				m.Start = 0
			}
			p.Matches = append(p.Matches, m)
			ids[m.PatternID] = true
		}
		if p.Kind == "" && chunk.Stream.Protocol == "TCP" {
			start := len(b) - snap.tail
			if start < 0 {
				start = 0
			}
			state.Tails[idx] = append([]byte(nil), b[start:]...)
		}
	}
	chunk.Stream.Patterns = []int{}
	for id := range ids {
		chunk.Stream.Patterns = append(chunk.Stream.Patterns, id)
	}
}

type processorState struct {
	key        string
	match      MatchState
	decoder    Decoder
	used       int
	touched    time.Time
	incomplete bool
	sequence   int64
}
type Processor struct {
	catalog             *Catalog
	states              map[string]*list.Element
	lru                 *list.List
	bytes               int
	MaxBytes, MaxStates int
	Evictions           int64
	DecodeErrors        int64
}

func NewProcessor(c *Catalog) *Processor {
	return &Processor{catalog: c, states: map[string]*list.Element{}, lru: list.New(), MaxBytes: 64 << 20, MaxStates: 4096}
}
func (p *Processor) remove(e *list.Element) {
	v := e.Value.(*processorState)
	p.bytes -= v.used
	delete(p.states, v.key)
	p.lru.Remove(e)
}
func (p *Processor) Size() int { return p.bytes }
func (p *Processor) Process(c *model.Chunk) {
	e := p.states[c.Stream.Key]
	if e == nil {
		e = p.lru.PushBack(&processorState{key: c.Stream.Key})
		p.states[c.Stream.Key] = e
	}
	p.lru.MoveToBack(e)
	v := e.Value.(*processorState)
	p.bytes -= v.used
	v.touched = time.Now()
	if c.Sequence > 0 && c.Sequence != v.sequence+1 {
		c.Stream.Incomplete = true
		v.decoder.Reset()
		v.match = MatchState{}
	}
	v.sequence = c.Sequence
	// A sticky stream gap invalidates protocol state once. Raw matching also resets
	// at explicit packet gaps; no fabricated continuity across missing bytes.
	if c.Stream.Incomplete && !v.incomplete {
		v.decoder.Reset()
		v.match = MatchState{}
		v.incomplete = true
	}
	svc, _ := p.catalog.Service(c.Stream.Service)
	c.Decoded = svc.HTTP || svc.WebSockets || svc.URLDecode
	raw := c.Packets
	if svc.HTTP || svc.WebSockets || svc.URLDecode {
		decodedBytes := 0
		for _, packet := range raw {
			if decodedBytes >= MessageLimit {
				c.Stream.Incomplete = true
				p.DecodeErrors++
				v.decoder.Reset()
				break
			}
			if packet.Kind != "" {
				continue
			}
			if packet.Incomplete {
				v.decoder.Reset()
			}
			decoded := v.decoder.Feed(packet, svc, false)
			for _, x := range decoded {
				decodedBytes += len(x.Content)
			}
			c.Packets = append(c.Packets, decoded...)
		}
		if c.Closed {
			for _, incoming := range []bool{true, false} {
				c.Packets = append(c.Packets, v.decoder.Feed(model.Packet{Incoming: incoming, Timestamp: c.Stream.End}, svc, true)...)
			}
		}
		c.Stream.UserAgentHash = v.decoder.UserAgentHash
	}
	for _, packet := range c.Packets {
		if packet.Kind == "decode-error" {
			c.Stream.Incomplete = true
			p.DecodeErrors++
		}
	}
	p.catalog.AnalyzeStreaming(c, &v.match)
	v.used = cap(v.match.Tails[0]) + cap(v.match.Tails[1]) + v.decoder.Size() + 512
	p.bytes += v.used
	if c.Closed {
		p.remove(e)
	}
	for p.lru.Len() > p.MaxStates || p.bytes > p.MaxBytes {
		p.remove(p.lru.Front())
		p.Evictions++
	}
}

func utf16lenValid(b []byte) int { return utf16len(strings.ToValidUTF8(string(b), "\uFFFD")) }

func (p *Processor) Forget(key string) {
	if e := p.states[key]; e != nil {
		p.remove(e)
	}
}
