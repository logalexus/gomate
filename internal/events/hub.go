package events

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// Events invalidate snapshots, rather than enqueueing every traffic record.
// A slow client consumes at most one pending notification.
type Hub struct {
	mu        sync.Mutex
	clients   map[chan struct{}]struct{}
	anomalies anomalyFeed
}

func New() *Hub { return &Hub{clients: map[chan struct{}]struct{}{}} }
func (h *Hub) Notify() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		select {
		case c <- struct{}{}:
		default:
		}
	}
}
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	if len(h.clients) >= 64 {
		h.mu.Unlock()
		http.Error(w, "too many event clients", 503)
		return
	}
	ch := make(chan struct{}, 1)
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	defer func() { h.mu.Lock(); delete(h.clients, ch); h.mu.Unlock() }()
	conn, e := websocket.Accept(w, r, nil)
	if e != nil {
		return
	}
	defer conn.CloseNow()
	ctx := conn.CloseRead(r.Context())
	select {
	case ch <- struct{}{}:
	default:
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			select {
			case <-ch:
			default:
				continue
			}
			send, cancel := context.WithTimeout(ctx, 2*time.Second)
			e = wsjson.Write(send, conn, map[string]any{"type": "INVALIDATE"})
			cancel()
			if e != nil {
				return
			}
		}
	}
}
