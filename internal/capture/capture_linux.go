//go:build linux

package capture

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/gopacket/gopacket/afpacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"gomate/internal/flow"
	"gomate/internal/metrics"
)

func File(ctx context.Context, path string, e *flow.Engine, m *metrics.Metrics) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r, err := pcapgo.NewReader(f)
	if err != nil {
		return err
	}
	m.CaptureRunning.Store(true)
	defer m.CaptureRunning.Store(false)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		data, ci, err := r.ReadPacketData()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		e.OfferContext(ctx, data, ci, r.LinkType())
	}
}

type Source struct{ h *afpacket.TPacket }

func Prepare(device string) (*Source, error) {
	if device == "any" {
		return nil, fmt.Errorf("choose a canonical Ethernet interface; 'any' can duplicate Docker traffic")
	}
	h, err := afpacket.NewTPacket(afpacket.OptInterface(device), afpacket.OptTPacketVersion(afpacket.TPacketVersion3), afpacket.OptFrameSize(65536), afpacket.OptBlockSize(1<<20), afpacket.OptNumBlocks(8), afpacket.OptPollTimeout(100*time.Millisecond))
	if err != nil {
		return nil, err
	}
	filter, _ := ServiceFilter(nil)
	if err = h.SetBPF(filter); err != nil {
		h.Close()
		return nil, err
	}
	return &Source{h: h}, nil
}
func (s *Source) Close() { s.h.Close() }
func Live(ctx context.Context, device string, e *flow.Engine, m *metrics.Metrics) error {
	source, err := Prepare(device)
	if err != nil {
		return err
	}
	defer source.Close()
	return source.Run(ctx, e, m)
}
func (source *Source) Run(ctx context.Context, e *flow.Engine, m *metrics.Metrics) error {
	var err error
	h := source.h
	activated := time.Now()
	if err = e.SetPortHook(func(ports []int) error {
		filter, err := ServiceFilter(ports)
		if err != nil {
			return err
		}
		return h.SetBPF(filter)
	}); err != nil {
		return err
	}
	defer e.SetPortHook(nil)
	m.CaptureRunning.Store(true)
	defer m.CaptureRunning.Store(false)
	lastStats := time.Now()
	var lastDrops int64
	for ctx.Err() == nil {
		data, ci, err := h.ZeroCopyReadPacketData()
		if err != nil && !errors.Is(err, afpacket.ErrTimeout) {
			return err
		}
		if err == nil && !ci.Timestamp.Before(activated) {
			e.Offer(data, ci, layers.LinkTypeEthernet)
		}
		if time.Since(lastStats) >= time.Second {
			_, stats, err := h.SocketStats()
			if err != nil {
				return err
			}
			drops := int64(stats.Drops())
			e.CaptureLoss(drops - lastDrops)
			lastDrops = drops
			lastStats = time.Now()
		}
	}
	return ctx.Err()
}
