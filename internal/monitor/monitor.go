package monitor

import (
	"context"
	"encoding/json"
	"gomate/internal/dockerui"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const Interval = 5 * time.Second
const historySize = 720
const maxContainers = 128

// Null means unavailable or warming up, never fabricated zero consumption.
type Point struct {
	Time          int64    `json:"time"`
	CPU           *float64 `json:"cpu"`
	MemoryUsed    *float64 `json:"memoryUsed"`
	MemoryLimit   *float64 `json:"memoryLimit"`
	RX            *float64 `json:"rx"`
	TX            *float64 `json:"tx"`
	Read          *float64 `json:"read"`
	Write         *float64 `json:"write"`
	DiskUsed      *float64 `json:"diskUsed,omitempty"`
	DiskTotal     *float64 `json:"diskTotal,omitempty"`
	DiskAvailable *float64 `json:"diskAvailable,omitempty"`
	Pids          uint64   `json:"pids"`
}
type Container struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Image string `json:"image"`
	State string `json:"state"`
	Point Point  `json:"point"`
	Error string `json:"error,omitempty"`
}
type Response struct {
	SampledAt        int64       `json:"sampledAt"`
	IntervalSeconds  int         `json:"intervalSeconds"`
	RetentionMinutes int         `json:"retentionMinutes"`
	Host             Host        `json:"host"`
	HostHistory      []Point     `json:"hostHistory"`
	Containers       []Container `json:"containers"`
	SelectedID       string      `json:"selectedId"`
	ContainerHistory []Point     `json:"containerHistory"`
	Error            string      `json:"error,omitempty"`
	Truncated        bool        `json:"truncated"`
}
type Source interface {
	List(context.Context) ([]dockerui.Container, error)
	Stats(context.Context, string) (dockerui.Stats, error)
}
type ring struct {
	points      [historySize]Point
	next, count int
}

func (r *ring) add(p Point) {
	r.points[r.next] = p
	r.next = (r.next + 1) % historySize
	if r.count < historySize {
		r.count++
	}
}
func (r *ring) since(cutoff int64) []Point {
	out := make([]Point, 0, r.count)
	for i := 0; i < r.count; i++ {
		p := r.points[(r.next-r.count+i+historySize)%historySize]
		if p.Time >= cutoff {
			out = append(out, p)
		}
	}
	return out
}

type previous struct {
	stats dockerui.Stats
	at    time.Time
}
type Monitor struct {
	source      Source
	proc, root  string
	mu          sync.RWMutex
	current     Response
	hostHistory ring
	histories   map[string]*ring
	// Counters are owned exclusively by Run, not by request handlers.
	prev     map[string]previous
	hostPrev hostCounters
	hostAt   time.Time
}

func New(source Source, proc, root string) *Monitor {
	return &Monitor{source: source, proc: proc, root: root, histories: make(map[string]*ring), prev: make(map[string]previous)}
}
func (m *Monitor) Run(ctx context.Context) {
	m.collect(ctx)
	ticker := time.NewTicker(Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.collect(ctx)
		}
	}
}
func ptr(v float64) *float64 { return &v }
func rate(current, prev uint64, seconds float64) *float64 {
	if seconds <= 0 || current < prev {
		return nil
	}
	return ptr(float64(current-prev) / seconds)
}
func network(s dockerui.Stats) (rx, tx uint64) {
	for _, n := range s.Networks {
		rx += n.RX
		tx += n.TX
	}
	return
}
func block(s dockerui.Stats) (read, write uint64) {
	for _, n := range s.Block.Entries {
		switch strings.ToLower(n.Op) {
		case "read":
			read += n.Value
		case "write":
			write += n.Value
		}
	}
	return
}
func containerPoint(s dockerui.Stats, prev previous, now time.Time) Point {
	p := Point{Time: now.UnixMilli(), Pids: s.Pids.Current}
	if s.Memory.Limit > 0 {
		cache, ok := s.Memory.Stats["total_inactive_file"]
		if !ok {
			cache = s.Memory.Stats["inactive_file"]
		}
		usage := s.Memory.Usage
		if cache < usage {
			usage -= cache
		}
		p.MemoryUsed, p.MemoryLimit = ptr(float64(usage)), ptr(float64(s.Memory.Limit))
	}
	if prev.at.IsZero() {
		return p
	}
	// Counter resets/restarts invalidate all rates until a new baseline exists.
	if s.CPU.Usage.Total < prev.stats.CPU.Usage.Total || (!s.Read.IsZero() && !s.Read.After(prev.stats.Read)) {
		return p
	}
	if s.CPU.System > prev.stats.CPU.System && s.CPU.CPUs > 0 {
		p.CPU = ptr(float64(s.CPU.Usage.Total-prev.stats.CPU.Usage.Total) / float64(s.CPU.System-prev.stats.CPU.System) * float64(s.CPU.CPUs) * 100)
	}
	seconds := now.Sub(prev.at).Seconds()
	if !s.Read.IsZero() && !prev.stats.Read.IsZero() {
		seconds = s.Read.Sub(prev.stats.Read).Seconds()
	}
	if len(s.Networks) > 0 && len(prev.stats.Networks) > 0 {
		rx, tx := network(s)
		prx, ptx := network(prev.stats)
		p.RX, p.TX = rate(rx, prx, seconds), rate(tx, ptx, seconds)
	}
	if s.Block.Entries != nil && prev.stats.Block.Entries != nil {
		r, w := block(s)
		pr, pw := block(prev.stats)
		p.Read, p.Write = rate(r, pr, seconds), rate(w, pw, seconds)
	}
	return p
}
func (m *Monitor) collect(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, 4*time.Second)
	defer cancel()
	now := time.Now()
	h, c, err := readHost(m.proc, m.root, now)
	if err != nil {
		h.Error = err.Error()
		m.hostAt = time.Time{}
	} else {
		if !m.hostAt.IsZero() {
			if c.total > m.hostPrev.total && c.idle >= m.hostPrev.idle && c.idle-m.hostPrev.idle <= c.total-m.hostPrev.total {
				h.CPU = ptr(100 * (1 - float64(c.idle-m.hostPrev.idle)/float64(c.total-m.hostPrev.total)))
			}
			if h.NetworkError == "" && strings.Join(c.networks, ",") == strings.Join(m.hostPrev.networks, ",") {
				h.RX, h.TX = rate(c.rx, m.hostPrev.rx, now.Sub(m.hostAt).Seconds()), rate(c.tx, m.hostPrev.tx, now.Sub(m.hostAt).Seconds())
			}
		}
		m.hostPrev, m.hostAt = c, now
	}
	result := Response{SampledAt: now.UnixMilli(), IntervalSeconds: 5, RetentionMinutes: 60, Host: h, Containers: []Container{}}
	list, err := m.source.List(ctx)
	if err != nil {
		result.Error = "Docker недоступен: показатели контейнеров не обновлены"
	} else {
		sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
		if len(list) > maxContainers {
			list = list[:maxContainers]
			result.Truncated = true
		}
		result.Containers = make([]Container, len(list))
		raw := make([]dockerui.Stats, len(list))
		valid := make([]bool, len(list))
		jobs := make(chan int, len(list))
		for i := range list {
			jobs <- i
		}
		close(jobs)
		var wg sync.WaitGroup
		for worker := 0; worker < 4; worker++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range jobs {
					c := list[i]
					v := Container{ID: c.ID, Name: c.Name, Image: c.Image, State: c.State, Point: Point{Time: now.UnixMilli()}}
					if c.State == "running" || c.State == "paused" {
						s, e := m.source.Stats(ctx, c.ID)
						if e != nil {
							v.Error = "Метрики недоступны"
						} else {
							raw[i] = s
							valid[i] = true
							v.Point = containerPoint(s, m.prev[c.ID], now)
						}
					}
					result.Containers[i] = v
				}
			}()
		}
		wg.Wait()
		newPrev := make(map[string]previous, len(list))
		for i, c := range list {
			if valid[i] {
				newPrev[c.ID] = previous{raw[i], now}
			}
		}
		m.prev = newPrev
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hostHistory.add(h.Point)
	if result.Error != "" {
		// Preserve identities, but append gaps rather than stale/zero measurements.
		result.Containers = append([]Container(nil), m.current.Containers...)
		for i := range result.Containers {
			result.Containers[i].Point = Point{Time: now.UnixMilli()}
			result.Containers[i].Error = "Docker недоступен"
		}
		m.prev = make(map[string]previous)
	}
	active := make(map[string]bool, len(result.Containers))
	for _, c := range result.Containers {
		active[c.ID] = true
		r := m.histories[c.ID]
		if r == nil {
			r = &ring{}
			m.histories[c.ID] = r
		}
		r.add(c.Point)
	}
	for id := range m.histories {
		if !active[id] {
			delete(m.histories, id)
		}
	}
	m.current = result
}
func (m *Monitor) snapshot(minutes int, id string) Response {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r := m.current
	cutoff := time.Now().Add(-time.Duration(minutes) * time.Minute).UnixMilli()
	r.HostHistory = m.hostHistory.since(cutoff)
	r.SelectedID = id
	r.ContainerHistory = []Point{}
	if h := m.histories[id]; h != nil {
		r.ContainerHistory = h.since(cutoff)
	}
	if r.SampledAt == 0 {
		r.Error = "Мониторинг запускается"
		r.Containers = []Container{}
	}
	return r
}
func (m *Monitor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != "GET" {
		w.WriteHeader(405)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}
	minutes, err := strconv.Atoi(r.URL.Query().Get("minutes"))
	if r.URL.Query().Get("minutes") == "" {
		minutes = 15
		err = nil
	}
	id := r.URL.Query().Get("container")
	if err != nil || (minutes != 5 && minutes != 15 && minutes != 60) || (id != "" && !dockerui.ValidID(id)) {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "Некорректный период или контейнер"})
		return
	}
	json.NewEncoder(w).Encode(m.snapshot(minutes, id))
}
