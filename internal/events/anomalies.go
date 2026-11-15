package events

import (
	"crypto/rand"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"gomate/internal/anomaly"
)

const anomalyFeedSize = 1024

type AnomalyNotice struct {
	StreamID int64   `json:"streamId"`
	Service  int     `json:"service"`
	Score    float64 `json:"score"`
}

type AnomalyNotifications struct {
	Cursor string          `json:"cursor"`
	Items  []AnomalyNotice `json:"items"`
	Reset  bool            `json:"reset"`
	Gap    bool            `json:"gap"`
}

type anomalyFeed struct {
	mu    sync.Mutex
	epoch string
	seq   uint64
	items [anomalyFeedSize]AnomalyNotice
}

func (f *anomalyFeed) init() {
	if f.epoch == "" {
		f.epoch = fmt.Sprintf("%x", randomEpoch())
	}
}

func randomEpoch() [16]byte {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return b
}

// PublishAnomalies is called only after a successful database flush. The ring
// bounds memory independently of traffic volume and the number of browsers.
func (h *Hub) PublishAnomalies(rows []anomaly.Result) {
	f := &h.anomalies
	f.mu.Lock()
	defer f.mu.Unlock()
	f.init()
	for _, r := range rows {
		f.items[f.seq%anomalyFeedSize] = AnomalyNotice{r.StreamID, r.Service, r.Score}
		f.seq++
	}
}

// An empty cursor establishes a baseline without replaying history. A cursor
// from a previous process also resets; overflow returns the retained tail.
func (h *Hub) AnomalyNotifications(cursor string) AnomalyNotifications {
	f := &h.anomalies
	f.mu.Lock()
	defer f.mu.Unlock()
	f.init()
	out := AnomalyNotifications{Cursor: fmt.Sprintf("%s:%d", f.epoch, f.seq), Items: []AnomalyNotice{}}
	if cursor == "" {
		return out
	}
	epoch, raw, ok := strings.Cut(cursor, ":")
	seq, err := strconv.ParseUint(raw, 10, 64)
	if !ok || epoch != f.epoch || err != nil || seq > f.seq {
		out.Reset = true
		return out
	}
	if f.seq-seq > anomalyFeedSize {
		out.Gap = true
		seq = f.seq - anomalyFeedSize
	}
	for ; seq < f.seq; seq++ {
		out.Items = append(out.Items, f.items[seq%anomalyFeedSize])
	}
	return out
}
