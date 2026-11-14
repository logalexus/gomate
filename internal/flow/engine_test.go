package flow

import (
	"bytes"
	"encoding/binary"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"gomate/internal/analyze"
	"gomate/internal/config"
	"gomate/internal/metrics"
	"gomate/internal/model"
)

func testConfig() config.Config {
	return config.Config{Shards: 2, QueuePackets: 64, QueueBytes: 1 << 20, MaxFlows: 128, MaxFlowBytes: 65536, FlushInterval: time.Second, IdleTimeout: 40 * time.Second, LocalIPs: []netip.Addr{netip.MustParseAddr("10.0.0.2")}}
}
func wire(t testing.TB, reverse bool, seq uint32, syn, ack, rst bool, payload string) []byte {
	t.Helper()
	src, dst := net.IPv4(10, 0, 0, 1), net.IPv4(10, 0, 0, 2)
	sp, dp := layers.TCPPort(12345), layers.TCPPort(80)
	if reverse {
		src, dst = dst, src
		sp, dp = dp, sp
	}
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{0, 1, 2, 3, 4, 5}, DstMAC: net.HardwareAddr{0, 1, 2, 3, 4, 6}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, TTL: 64, SrcIP: src, DstIP: dst, Protocol: layers.IPProtocolTCP}
	tcp := &layers.TCP{SrcPort: sp, DstPort: dp, Seq: seq, SYN: syn, ACK: ack, RST: rst, Window: 65535}
	tcp.SetNetworkLayerForChecksum(ip)
	b := gopacket.NewSerializeBuffer()
	if err := gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, tcp, gopacket.Payload(payload)); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestReassemblyReorderRetransmitAndSplitFlag(t *testing.T) {
	c := analyze.NewCatalog()
	c.Replace([]model.Service{{Port: 80}}, []model.Pattern{{ID: 1, Name: "flag", Value: "FLAG", Color: "#ff0000", Enabled: true, SearchType: "SUBSTRING", Action: "FIND", Direction: "BOTH"}})
	var mu sync.Mutex
	var chunks []*model.Chunk
	m := &metrics.Metrics{}
	e := New(testConfig(), c, m, false, func(v *model.Chunk) bool {
		c.Analyze(v)
		mu.Lock()
		chunks = append(chunks, v)
		mu.Unlock()
		return true
	})
	frames := [][]byte{wire(t, false, 100, true, false, false, ""), wire(t, true, 200, true, true, false, ""), wire(t, false, 104, false, true, false, "G"), wire(t, false, 101, false, true, false, "FLA"), wire(t, false, 101, false, true, false, "FLAG"), wire(t, true, 201, false, true, false, "OK"), wire(t, false, 105, false, true, true, ""), wire(t, true, 203, false, true, true, "")}
	for i, b := range frames {
		e.Offer(b, gopacket.CaptureInfo{Timestamp: time.Unix(100, int64(i)*int64(time.Millisecond)), CaptureLength: len(b), Length: len(b)}, layers.LinkTypeEthernet)
	}
	e.Close()
	var incoming, outgoing []byte
	found := false
	for _, v := range chunks {
		if v.Stream.Incomplete {
			t.Fatal("complete trace marked incomplete")
		}
		for _, p := range v.Packets {
			if p.Incoming {
				incoming = append(incoming, p.Content...)
			} else {
				outgoing = append(outgoing, p.Content...)
			}
			found = found || len(p.Matches) > 0
		}
	}
	if string(incoming) != "FLAG" || string(outgoing) != "OK" || !found {
		t.Fatalf("incoming=%q outgoing=%q found=%v", incoming, outgoing, found)
	}
	if m.QueueBytes.Load() != 0 || m.ActiveFlows.Load() != 0 {
		t.Fatalf("leaked resource accounting: %v", m.Snapshot())
	}
}
func TestOpenConnectionPublishedBeforeClose(t *testing.T) {
	cfg := testConfig()
	cfg.FlushInterval = 20 * time.Millisecond
	c := analyze.NewCatalog()
	c.Replace([]model.Service{{Port: 80}}, nil)
	out := make(chan *model.Chunk, 4)
	e := New(cfg, c, &metrics.Metrics{}, true, func(v *model.Chunk) bool { out <- v; return true })
	defer e.Close()
	for _, b := range [][]byte{wire(t, false, 100, true, false, false, ""), wire(t, false, 101, false, true, false, "still open")} {
		e.Offer(b, gopacket.CaptureInfo{Timestamp: time.Now()}, layers.LinkTypeEthernet)
	}
	select {
	case v := <-out:
		if !bytes.Equal(v.Packets[0].Content, []byte("still open")) {
			t.Fatal("wrong data")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stream waited for TCP termination")
	}
}
func TestMemoryBudgetRejectsAdmission(t *testing.T) {
	cfg := testConfig()
	cfg.QueueBytes = 1
	c := analyze.NewCatalog()
	c.Replace([]model.Service{{Port: 80}}, nil)
	m := &metrics.Metrics{}
	e := New(cfg, c, m, true, func(*model.Chunk) bool { return true })
	b := wire(t, false, 1, false, true, false, "payload")
	if e.Offer(b, gopacket.CaptureInfo{Timestamp: time.Now()}, layers.LinkTypeEthernet) {
		t.Fatal("over-budget packet accepted")
	}
	e.Close()
	if m.QueueDrops.Load() != 1 || m.QueueBytes.Load() != 0 {
		t.Fatal(m.Snapshot())
	}
}

func TestFinalACKDoesNotCreateEmptyConnection(t *testing.T) {
	c := analyze.NewCatalog()
	c.Replace([]model.Service{{Port: 80}}, nil)
	m := &metrics.Metrics{}
	e := New(testConfig(), c, m, true, func(*model.Chunk) bool { return true })
	defer e.Close()
	b := wire(t, false, 100, false, true, false, "")
	e.Offer(b, gopacket.CaptureInfo{Timestamp: time.Now()}, layers.LinkTypeEthernet)
	deadline := time.Now().Add(time.Second)
	for m.QueueBytes.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if m.ActiveFlows.Load() != 0 {
		t.Fatal("pure ACK resurrected a flow")
	}
}
func TestMidstreamCaptureMarkedIncomplete(t *testing.T) {
	c := analyze.NewCatalog()
	c.Replace([]model.Service{{Port: 80}}, nil)
	out := make(chan *model.Chunk, 1)
	e := New(testConfig(), c, &metrics.Metrics{}, false, func(v *model.Chunk) bool { out <- v; return true })
	e.Offer(wire(t, false, 99, false, true, false, "midstream"), gopacket.CaptureInfo{Timestamp: time.Now()}, layers.LinkTypeEthernet)
	e.Close()
	select {
	case v := <-out:
		if !v.Stream.Incomplete {
			t.Fatal("gap not reported")
		}
	default:
		t.Fatal("missing data")
	}
}

func TestMalformedApplicationPayloadStillCaptured(t *testing.T) {
	c := analyze.NewCatalog()
	c.Replace([]model.Service{{Port: 53}}, nil)
	out := make(chan *model.Chunk, 1)
	e := New(testConfig(), c, &metrics.Metrics{}, false, func(v *model.Chunk) bool { out <- v; return true })
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{0, 1, 2, 3, 4, 5}, DstMAC: net.HardwareAddr{0, 1, 2, 3, 4, 6}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, TTL: 64, SrcIP: net.IPv4(10, 0, 0, 1), DstIP: net.IPv4(10, 0, 0, 2), Protocol: layers.IPProtocolUDP}
	udp := &layers.UDP{SrcPort: 12345, DstPort: 53}
	udp.SetNetworkLayerForChecksum(ip)
	b := gopacket.NewSerializeBuffer()
	if err := gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, udp, gopacket.Payload("bad")); err != nil {
		t.Fatal(err)
	}
	if !e.Offer(b.Bytes(), gopacket.CaptureInfo{Timestamp: time.Now()}, layers.LinkTypeEthernet) {
		t.Fatal("application parse error dropped valid UDP")
	}
	e.Close()
	select {
	case v := <-out:
		if string(v.Packets[0].Content) != "bad" {
			t.Fatal("payload changed")
		}
	default:
		t.Fatal("no payload")
	}
}

func TestSYNFloodRespectsActiveFlowLimit(t *testing.T) {
	cfg := testConfig()
	cfg.Shards = 1
	cfg.MaxFlows = 8
	cfg.QueuePackets = 256
	c := analyze.NewCatalog()
	c.Replace([]model.Service{{Port: 80}}, nil)
	m := &metrics.Metrics{}
	e := New(cfg, c, m, true, func(*model.Chunk) bool { return true })
	defer e.Close()
	for i := 0; i < 100; i++ {
		b := wire(t, false, 1, true, false, false, "")
		binary.BigEndian.PutUint16(b[34:36], uint16(10000+i))
		e.Offer(b, gopacket.CaptureInfo{Timestamp: time.Now()}, layers.LinkTypeEthernet)
	}
	deadline := time.Now().Add(time.Second)
	for m.QueueBytes.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if m.ActiveFlows.Load() > 8 || m.FlowDrops.Load() == 0 {
		t.Fatalf("flow budget not enforced: %v", m.Snapshot())
	}
}

func FuzzPacketAdmission(f *testing.F) {
	f.Add([]byte{0, 1, 2})
	f.Add(wire(f, false, 1, true, false, false, ""))
	cfg := testConfig()
	cfg.MaxFlows = 16
	c := analyze.NewCatalog()
	c.Replace([]model.Service{{Port: 80}}, nil)
	e := New(cfg, c, &metrics.Metrics{}, true, func(*model.Chunk) bool { return true })
	f.Cleanup(e.Close)
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 65536 {
			return
		}
		e.Offer(b, gopacket.CaptureInfo{Timestamp: time.Now(), CaptureLength: len(b), Length: len(b)}, layers.LinkTypeEthernet)
	})
}

func TestTupleReuseStartsNewConnection(t *testing.T) {
	cat := analyze.NewCatalog()
	cat.Replace([]model.Service{{Port: 80}}, nil)
	m := &metrics.Metrics{}
	var chunks []*model.Chunk
	var mu sync.Mutex
	e := New(testConfig(), cat, m, false, func(c *model.Chunk) bool { mu.Lock(); defer mu.Unlock(); chunks = append(chunks, c); return true })
	frames := [][]byte{wire(t, false, 100, true, false, false, ""), wire(t, false, 101, false, true, false, "old"), wire(t, false, 1000, true, false, false, ""), wire(t, false, 1001, false, true, false, "new")}
	for i, b := range frames {
		e.Offer(b, gopacket.CaptureInfo{Timestamp: time.Unix(100, int64(i)*int64(time.Millisecond)), CaptureLength: len(b), Length: len(b)}, layers.LinkTypeEthernet)
	}
	e.Close()
	payloads := map[string]string{}
	for _, c := range chunks {
		for _, p := range c.Packets {
			payloads[c.Stream.Key] += string(p.Content)
		}
	}
	if len(payloads) != 2 {
		t.Fatal(payloads)
	}
	for _, v := range payloads {
		if v != "old" && v != "new" {
			t.Fatal(payloads)
		}
	}
	if m.ActiveBytes.Load() != 0 {
		t.Fatal("active budget leak", m.Snapshot())
	}
}
func TestGlobalActiveBytesBudget(t *testing.T) {
	cfg := testConfig()
	cfg.ActiveBytes = 4
	cat := analyze.NewCatalog()
	cat.Replace([]model.Service{{Port: 80}}, nil)
	m := &metrics.Metrics{}
	e := New(cfg, cat, m, false, func(c *model.Chunk) bool { return true })
	b := wire(t, false, 1, true, false, false, "too large")
	e.Offer(b, gopacket.CaptureInfo{Timestamp: time.Now(), Length: len(b), CaptureLength: len(b)}, layers.LinkTypeEthernet)
	e.Close()
	if m.ActiveBytes.Load() != 0 || m.FlowDrops.Load() == 0 {
		t.Fatal(m.Snapshot())
	}
}
