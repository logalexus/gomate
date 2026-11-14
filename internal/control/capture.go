package control

import (
	"context"
	"fmt"
	"gomate/internal/analyze"
	"gomate/internal/capture"
	"gomate/internal/config"
	"gomate/internal/flow"
	"gomate/internal/metrics"
	"gomate/internal/model"
	"net"
	"net/netip"
	"reflect"
	"sort"
	"sync"
	"time"
)

type Interface struct {
	Name      string   `json:"name"`
	Addresses []string `json:"addresses"`
	MAC       string   `json:"mac"`
	Up        bool     `json:"up"`
	Supported bool     `json:"supported"`
	Reason    string   `json:"reason,omitempty"`
}

func Interfaces() ([]Interface, error) {
	all, e := net.Interfaces()
	if e != nil {
		return nil, e
	}
	out := []Interface{}
	for _, i := range all {
		v := Interface{Name: i.Name, MAC: i.HardwareAddr.String(), Up: i.Flags&net.FlagUp != 0, Addresses: []string{}}
		addresses, e := i.Addrs()
		if e != nil {
			return nil, e
		}
		for _, a := range addresses {
			if p, e := netip.ParsePrefix(a.String()); e == nil {
				v.Addresses = append(v.Addresses, p.Addr().Unmap().String())
			}
		}
		v.Supported = v.Up && len(i.HardwareAddr) == 6 && i.Flags&net.FlagLoopback == 0
		if !v.Up {
			v.Reason = "Interface is down"
		} else if !v.Supported {
			v.Reason = "This capture adapter requires Ethernet"
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
func Validate(v model.RuntimeSettings) error {
	if v.CaptureEnabled && v.Interface == "" {
		return fmt.Errorf("select a capture interface")
	}
	for _, x := range v.LocalIPs {
		if _, e := netip.ParseAddr(x); e != nil {
			return fmt.Errorf("invalid local IP: %s", x)
		}
	}
	if len(v.LocalIPs) > 128 {
		return fmt.Errorf("maximum 128 local addresses")
	}
	for _, sec := range []int64{v.PacketRetentionSeconds, v.SessionRetentionSeconds} {
		if sec != 0 && (sec < 60 || sec > 90*86400) {
			return fmt.Errorf("retention must be disabled (0) or between 1 minute and 90 days")
		}
	}
	if v.PacketRetentionSeconds > 0 && v.SessionRetentionSeconds > 0 && v.PacketRetentionSeconds > v.SessionRetentionSeconds {
		return fmt.Errorf("packet retention cannot exceed session retention")
	}
	if v.CleanupIntervalSeconds < 5 || v.CleanupIntervalSeconds > 3600 {
		return fmt.Errorf("cleanup interval must be 5–3600 seconds")
	}
	return nil
}
func resolve(v model.RuntimeSettings) ([]netip.Addr, error) {
	all, e := Interfaces()
	if e != nil {
		return nil, e
	}
	var found *Interface
	for i := range all {
		if all[i].Name == v.Interface {
			found = &all[i]
			break
		}
	}
	if found == nil {
		return nil, fmt.Errorf("interface %q no longer exists", v.Interface)
	}
	if !found.Supported {
		return nil, fmt.Errorf("%s: %s", v.Interface, found.Reason)
	}
	addresses := v.LocalIPs
	if v.AutoIPs {
		addresses = found.Addresses
	}
	out := []netip.Addr{}
	for _, s := range addresses {
		a, e := netip.ParseAddr(s)
		if e != nil {
			return nil, e
		}
		out = append(out, a.Unmap())
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("interface has no addresses; specify local IPs manually")
	}
	return out, nil
}

type Status struct {
	Running   bool      `json:"running"`
	Interface string    `json:"interface"`
	LocalIPs  []string  `json:"localIPs"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"startedAt"`
}
type Source interface {
	Run(context.Context, *flow.Engine, *metrics.Metrics) error
	Close()
}
type Manager struct {
	op       sync.Mutex
	mu       sync.RWMutex
	settings model.RuntimeSettings
	status   Status
	cfg      config.Config
	catalog  *analyze.Catalog
	metrics  *metrics.Metrics
	submit   func(*model.Chunk) bool
	notify   func()
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
	prepare  func(string) (Source, error)
	resolve  func(model.RuntimeSettings) ([]netip.Addr, error)
}

func New(ctx context.Context, cfg config.Config, c *analyze.Catalog, m *metrics.Metrics, submit func(*model.Chunk) bool, notify func()) *Manager {
	return &Manager{ctx: ctx, cfg: cfg, catalog: c, metrics: m, submit: submit, notify: notify, prepare: func(name string) (Source, error) { return capture.Prepare(name) }, resolve: resolve}
}
func (m *Manager) Settings() model.RuntimeSettings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v := m.settings
	v.LocalIPs = append([]string{}, v.LocalIPs...)
	return v
}
func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v := m.status
	v.Running = m.metrics.CaptureRunning.Load()
	v.LocalIPs = append([]string{}, v.LocalIPs...)
	return v
}

// Apply is serialized with shutdown. Preparation failures leave the previous
// source running. Persistence succeeds before the old reader is drained.
func (m *Manager) Apply(ctx context.Context, v model.RuntimeSettings, persist func(context.Context, *model.RuntimeSettings) error) error {
	if e := Validate(v); e != nil {
		return e
	}
	m.op.Lock()
	defer m.op.Unlock()
	if m.ctx.Err() != nil {
		return m.ctx.Err()
	}
	old := m.Settings()
	status := m.Status()
	changed := old.CaptureEnabled != v.CaptureEnabled || old.Interface != v.Interface || old.AutoIPs != v.AutoIPs || !reflect.DeepEqual(old.LocalIPs, v.LocalIPs) || (v.CaptureEnabled && !status.Running)
	var source Source
	var ips []netip.Addr
	var e error
	if changed && v.CaptureEnabled {
		ips, e = m.resolve(v)
		if e != nil {
			return e
		}
		source, e = m.prepare(v.Interface)
		if e != nil {
			return fmt.Errorf("open capture interface: %w", e)
		}
	}
	if persist != nil {
		if e = persist(ctx, &v); e != nil {
			if source != nil {
				source.Close()
			}
			return e
		}
	}
	m.mu.Lock()
	m.settings = v
	m.mu.Unlock()
	if !changed {
		m.notify()
		return nil
	}
	m.stop()
	m.mu.Lock()
	m.status = Status{Interface: v.Interface, LocalIPs: []string{}}
	for _, ip := range ips {
		m.status.LocalIPs = append(m.status.LocalIPs, ip.String())
	}
	m.mu.Unlock()
	if source != nil {
		cfg := m.cfg
		cfg.Interface = v.Interface
		cfg.LocalIPs = ips
		engine := flow.New(cfg, m.catalog, m.metrics, true, m.submit)
		q, cancel := context.WithCancel(m.ctx)
		m.cancel = cancel
		m.done = make(chan struct{})
		done := m.done
		m.mu.Lock()
		m.status.StartedAt = time.Now().UTC()
		m.mu.Unlock()
		go func() {
			err := source.Run(q, engine, m.metrics)
			source.Close()
			engine.Close()
			m.mu.Lock()
			if err != nil && q.Err() == nil {
				m.status.Error = err.Error()
			}
			m.mu.Unlock()
			m.notify()
			close(done)
		}()
	}
	m.notify()
	return nil
}
func (m *Manager) stop() {
	if m.cancel != nil {
		m.cancel()
		<-m.done
		m.cancel = nil
		m.done = nil
	}
}
func (m *Manager) Close() { m.op.Lock(); defer m.op.Unlock(); m.stop() }
func (m *Manager) Startup(v model.RuntimeSettings) {
	if e := m.Apply(m.ctx, v, nil); e != nil {
		m.mu.Lock()
		m.settings = v
		m.status.Error = e.Error()
		m.mu.Unlock()
	}
}
