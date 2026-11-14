package flow

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/tcpassembly"
	"gomate/internal/analyze"
	"gomate/internal/config"
	"gomate/internal/metrics"
	"gomate/internal/model"
)

type frame struct {
	key                      string
	network                  gopacket.Flow
	tcp                      *layers.TCP
	payload                  []byte
	incoming                 bool
	service, ttl             int
	client, server, protocol string
	captured, seen           time.Time
	cost                     int64
	epoch                    int64
}
type Engine struct {
	cfg     config.Config
	catalog *analyze.Catalog
	metrics *metrics.Metrics
	shards  []*shard
	wg      sync.WaitGroup
	submit  func(*model.Chunk) bool
	live    bool
	prefix  string
	counter atomic.Uint64
}
type shard struct {
	engine      *Engine
	input       chan frame
	flows       map[string]*connection
	assembler   *tcpassembly.Assembler
	current     frame
	currentFlow *connection
	directions  int
	epoch       atomic.Int64
	lastSweep   time.Time
}
type connection struct {
	key             string
	retired         bool
	synSeen         bool
	synSeq          uint32
	sequence        int64
	meta            model.Stream
	packets         []model.Packet
	bytes           int64
	seen, lastFlush time.Time
	incomplete      bool
	epoch           int64
	dirs            [2]*direction
}
type direction struct {
	shard    *shard
	flow     *connection
	incoming bool
	done     bool
}

func New(cfg config.Config, catalog *analyze.Catalog, m *metrics.Metrics, live bool, submit func(*model.Chunk) bool) *Engine {
	if cfg.ActiveBytes == 0 {
		cfg.ActiveBytes = 64 << 20
	}
	b := make([]byte, 12)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	e := &Engine{cfg: cfg, catalog: catalog, metrics: m, live: live, submit: submit, prefix: hex.EncodeToString(b)}
	m.LastProcess.Store(time.Now().Unix())
	for i := 0; i < cfg.Shards; i++ {
		s := &shard{engine: e, input: make(chan frame, cfg.QueuePackets), flows: map[string]*connection{}}
		s.assembler = tcpassembly.NewAssembler(tcpassembly.NewStreamPool(s))
		s.assembler.MaxBufferedPagesTotal = 1024
		s.assembler.MaxBufferedPagesPerConnection = 64
		e.shards = append(e.shards, s)
		e.wg.Add(1)
		go s.run()
	}
	return e
}
func (e *Engine) ID() string { return fmt.Sprintf("%s-%d", e.prefix, e.counter.Add(1)) }
func (e *Engine) Close() {
	for _, s := range e.shards {
		close(s.input)
	}
	e.wg.Wait()
}

// Offer never waits for downstream work. The capture ring memory is copied only
// after admission. On loss an epoch marks affected shard streams incomplete.
func (e *Engine) Offer(raw []byte, ci gopacket.CaptureInfo, link layers.LinkType) bool {
	return e.OfferContext(context.Background(), raw, ci, link)
}

func (e *Engine) OfferContext(ctx context.Context, raw []byte, ci gopacket.CaptureInfo, link layers.LinkType) bool {
	e.metrics.Captured.Add(1)
	e.metrics.LastCapture.Store(time.Now().Unix())
	packet := gopacket.NewPacket(raw, link, gopacket.NoCopy)
	var src, dst netip.Addr
	ttl := 0
	if l := packet.Layer(layers.LayerTypeIPv4); l != nil {
		ip := l.(*layers.IPv4)
		if ip.FragOffset != 0 || ip.Flags&layers.IPv4MoreFragments != 0 {
			e.metrics.Unsupported.Add(1)
			return false
		}
		src, _ = netip.AddrFromSlice(ip.SrcIP)
		dst, _ = netip.AddrFromSlice(ip.DstIP)
		ttl = int(ip.TTL)
	} else if l := packet.Layer(layers.LayerTypeIPv6); l != nil {
		ip := l.(*layers.IPv6)
		if packet.Layer(layers.LayerTypeIPv6Fragment) != nil {
			e.metrics.Unsupported.Add(1)
			return false
		}
		src, _ = netip.AddrFromSlice(ip.SrcIP)
		dst, _ = netip.AddrFromSlice(ip.DstIP)
		ttl = int(ip.HopLimit)
	} else {
		e.metrics.Filtered.Add(1)
		return false
	}
	src = src.Unmap()
	dst = dst.Unmap()
	f := frame{captured: ci.Timestamp, seen: time.Now(), ttl: ttl, network: packet.NetworkLayer().NetworkFlow()}
	if !e.live {
		f.seen = ci.Timestamp
	}
	var sp, dp uint16
	if l := packet.Layer(layers.LayerTypeTCP); l != nil {
		t := *l.(*layers.TCP)
		if t.DataOffset < 5 || len(t.Contents) < 20 {
			e.metrics.Unsupported.Add(1)
			return false
		}
		f.tcp = &t
		f.payload = t.Payload
		sp = uint16(t.SrcPort)
		dp = uint16(t.DstPort)
		f.protocol = "TCP"
	} else if l := packet.Layer(layers.LayerTypeUDP); l != nil {
		u := l.(*layers.UDP)
		if len(u.Contents) < 8 {
			e.metrics.Unsupported.Add(1)
			return false
		}
		f.payload = u.Payload
		sp = uint16(u.SrcPort)
		dp = uint16(u.DstPort)
		f.protocol = "UDP"
	} else {
		e.metrics.Filtered.Add(1)
		return false
	}
	local := func(a netip.Addr) bool {
		for _, ip := range e.cfg.LocalIPs {
			if ip == a {
				return true
			}
		}
		return false
	}
	_, destService := e.catalog.Service(int(dp))
	_, sourceService := e.catalog.Service(int(sp))
	if local(dst) && destService {
		f.incoming = true
		f.service = int(dp)
	} else if local(src) && sourceService {
		f.service = int(sp)
	} else {
		e.metrics.Filtered.Add(1)
		return false
	}
	f.client = netip.AddrPortFrom(src, sp).String()
	f.server = netip.AddrPortFrom(dst, dp).String()
	if !f.incoming {
		f.client, f.server = f.server, f.client
	}
	vlan := ""
	for _, l := range packet.Layers() {
		if q, ok := l.(*layers.Dot1Q); ok {
			vlan += fmt.Sprintf("/%d", q.VLANIdentifier)
		}
	}
	f.key = f.protocol + vlan + "/" + f.client + "/" + f.server
	h := fnv.New32a()
	h.Write([]byte(f.key))
	s := e.shards[int(h.Sum32())%len(e.shards)]
	f.cost = int64(len(f.payload) + 128)
	for !metrics.Reserve(&e.metrics.QueueBytes, f.cost, e.cfg.QueueBytes) {
		if e.live || f.cost > e.cfg.QueueBytes || ctx.Err() != nil {
			e.metrics.QueueDrops.Add(1)
			s.epoch.Add(1)
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(time.Millisecond):
		}
	}
	f.epoch = s.epoch.Load()
	f.payload = append([]byte(nil), f.payload...)
	if f.tcp != nil {
		f.tcp.BaseLayer = layers.BaseLayer{Payload: f.payload}
	}
	if !e.live {
		select {
		case s.input <- f:
			e.metrics.Accepted.Add(1)
			return true
		case <-ctx.Done():
			e.metrics.QueueBytes.Add(-f.cost)
			return false
		}
	}
	select {
	case s.input <- f:
		e.metrics.Accepted.Add(1)
		return true
	default:
		e.metrics.QueueBytes.Add(-f.cost)
		e.metrics.QueueDrops.Add(1)
		s.epoch.Add(1)
		return false
	}
}
func (e *Engine) CaptureLoss(n int64) {
	if n <= 0 {
		return
	}
	e.metrics.CaptureDrops.Add(n)
	for _, s := range e.shards {
		s.epoch.Add(1)
	}
}

func (s *shard) run() {
	defer s.engine.wg.Done()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case f, ok := <-s.input:
			if !ok {
				s.assembler.FlushAll()
				for _, c := range s.flows {
					s.finish(c)
				}
				s.engine.metrics.ActiveFlows.Add(-int64(len(s.flows)))
				return
			}
			s.process(f)
			s.engine.metrics.QueueBytes.Add(-f.cost)
			s.engine.metrics.LastProcess.Store(time.Now().Unix())
			if !s.engine.live && (s.lastSweep.IsZero() || f.seen.Sub(s.lastSweep) >= time.Second) {
				s.sweep(f.seen)
			}
		case now := <-ticker.C:
			if s.engine.live {
				s.sweep(now)
			}
		}
	}
}
func (s *shard) process(f frame) {
	// Pure ACKs carry no reassembly data. In particular the final ACK after both
	// FINs must not resurrect an empty connection until the idle timeout.
	if f.tcp != nil && len(f.payload) == 0 && !f.tcp.SYN && !f.tcp.FIN && !f.tcp.RST {
		return
	}
	c := s.flows[f.key]
	if c != nil && f.tcp != nil && f.tcp.SYN && !f.tcp.ACK && (!c.synSeen || c.synSeq != f.tcp.Seq) {
		c.incomplete = true
		s.finish(c)
		c.retired = true
		delete(s.flows, f.key)
		s.engine.metrics.ActiveFlows.Add(-1)
		c = nil
	}
	if c == nil {
		if f.tcp != nil && !f.tcp.SYN && len(f.payload) == 0 {
			return
		}
		if len(s.flows) >= s.engine.cfg.MaxFlows/s.engine.cfg.Shards || s.directions >= 2*s.engine.cfg.MaxFlows/s.engine.cfg.Shards {
			s.engine.metrics.FlowDrops.Add(1)
			return
		}
		c = &connection{key: f.key, meta: model.Stream{Key: s.engine.ID(), Service: f.service, Protocol: f.protocol, Start: f.captured.UnixMilli(), End: f.captured.UnixMilli(), TTL: f.ttl, Client: f.client, Server: f.server}, seen: f.seen, lastFlush: f.seen, epoch: f.epoch}
		s.flows[f.key] = c
		s.engine.metrics.ActiveFlows.Add(1)
	}
	if f.tcp != nil && f.tcp.SYN && !f.tcp.ACK {
		c.synSeen = true
		c.synSeq = f.tcp.Seq
	}
	if c.epoch != f.epoch || f.epoch != s.epoch.Load() {
		c.incomplete = true
		c.epoch = s.epoch.Load()
	}
	c.seen = f.seen
	c.meta.End = f.captured.UnixMilli()
	s.current = f
	s.currentFlow = c
	if f.tcp != nil {
		// Synthetic endpoints include the connection generation and VLAN identity.
		// Endpoint storage is limited to 16 bytes; hash the full identity per direction.
		src := hashEndpoint(c.meta.Key + "/" + f.network.Src().String())
		dst := hashEndpoint(c.meta.Key + "/" + f.network.Dst().String())
		nf := gopacket.NewFlow(layers.EndpointIPv6, src, dst)
		s.assembler.AssembleWithTimestamp(nf, f.tcp, f.seen)
	} else {
		s.append(c, f.payload, f.incoming, f.captured, false)
	}
	if c.bytes >= int64(s.engine.cfg.MaxFlowBytes) || f.seen.Sub(c.lastFlush) >= s.engine.cfg.FlushInterval {
		s.flush(c)
	}
	if f.tcp != nil && c.dirs[0] != nil && c.dirs[1] != nil && c.dirs[0].done && c.dirs[1].done {
		s.finish(c)
		delete(s.flows, f.key)
		s.engine.metrics.ActiveFlows.Add(-1)
	}
}
func hashEndpoint(s string) []byte { h := fnv.New128a(); h.Write([]byte(s)); return h.Sum(nil) }
func (s *shard) New(_, _ gopacket.Flow) tcpassembly.Stream {
	d := &direction{shard: s, flow: s.currentFlow, incoming: s.current.incoming}
	idx := 0
	if d.incoming {
		idx = 1
	}
	d.flow.dirs[idx] = d
	s.directions++
	return d
}
func (d *direction) Reassembled(rs []tcpassembly.Reassembly) {
	if d.flow.retired {
		return
	}
	for _, r := range rs {
		if r.Skip != 0 {
			d.flow.incomplete = true
			d.shard.engine.metrics.Gaps.Add(1)
		}
		stamp := r.Seen
		d.shard.append(d.flow, r.Bytes, d.incoming, stamp, r.Skip != 0)
	}
}
func (d *direction) ReassemblyComplete() {
	if !d.done {
		d.done = true
		d.shard.directions--
	}
}
func (s *shard) append(c *connection, b []byte, incoming bool, t time.Time, gap bool) {
	for len(b) > 0 {
		room := s.engine.cfg.MaxFlowBytes - int(c.bytes)
		if room <= 0 {
			s.flush(c)
			room = s.engine.cfg.MaxFlowBytes
		}
		n := len(b)
		if n > room {
			n = room
		}
		if n > 16384 {
			n = 16384
		}
		if !metrics.Reserve(&s.engine.metrics.ActiveBytes, int64(n), s.engine.cfg.ActiveBytes) {
			s.flush(c)
			if !metrics.Reserve(&s.engine.metrics.ActiveBytes, int64(n), s.engine.cfg.ActiveBytes) {
				c.incomplete = true
				s.engine.metrics.FlowDrops.Add(1)
				return
			}
		}
		reserved := n
		if len(c.packets) > 0 && c.meta.Protocol == "TCP" && !gap && c.packets[len(c.packets)-1].Incoming == incoming && len(c.packets[len(c.packets)-1].Content) < 16384 {
			last := &c.packets[len(c.packets)-1]
			if n > 16384-len(last.Content) {
				n = 16384 - len(last.Content)
			}
			last.Content = append(last.Content, b[:n]...)
		} else {
			c.packets = append(c.packets, model.Packet{Timestamp: t.UnixMilli(), Incoming: incoming, Content: append([]byte(nil), b[:n]...), Incomplete: gap, Matches: []model.Match{}})
		}
		s.engine.metrics.ActiveBytes.Add(int64(n - reserved))
		c.bytes += int64(n)
		b = b[n:]
		if len(c.packets) >= 256 {
			s.flush(c)
		}
	}
}
func (s *shard) flush(c *connection) {
	if c.bytes == 0 {
		return
	}
	v := c.meta
	v.Incomplete = c.incomplete
	c.sequence++
	chunk := &model.Chunk{Sequence: c.sequence, Token: s.engine.ID(), Stream: v, Packets: c.packets, Bytes: c.bytes}
	if !s.engine.submit(chunk) {
		c.incomplete = true
	}
	s.engine.metrics.ActiveBytes.Add(-c.bytes)
	c.packets = nil
	c.bytes = 0
	c.lastFlush = c.seen
}
func (s *shard) sweep(now time.Time) {
	s.lastSweep = now
	s.current.captured = time.Time{}
	s.assembler.FlushOlderThan(now.Add(-s.engine.cfg.IdleTimeout))
	for key, c := range s.flows {
		if c.epoch != s.epoch.Load() {
			c.incomplete = true
			c.epoch = s.epoch.Load()
		}
		if now.Sub(c.lastFlush) >= s.engine.cfg.FlushInterval {
			s.flush(c)
			c.lastFlush = now
		}
		if now.Sub(c.seen) > s.engine.cfg.IdleTimeout {
			s.finish(c)
			delete(s.flows, key)
			s.engine.metrics.ActiveFlows.Add(-1)
		}
	}
}

func (s *shard) finish(c *connection) {
	if c.sequence == 0 && c.bytes == 0 {
		return
	}
	v := c.meta
	v.Incomplete = c.incomplete
	c.sequence++
	s.engine.submit(&model.Chunk{Sequence: c.sequence, Token: s.engine.ID(), Stream: v, Packets: c.packets, Bytes: c.bytes, Closed: true})
	s.engine.metrics.ActiveBytes.Add(-c.bytes)
	c.packets = nil
	c.bytes = 0
}

func (e *Engine) SetPortHook(h func([]int) error) error { return e.catalog.SetPortHook(h) }
