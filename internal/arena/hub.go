package arena

import (
	"context"
	"encoding/json"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"net/http"
	"sync"
	"time"
)

// Hub's zero value is ready to use. It has no database, capture locks or unbounded queues.
type Hub struct {
	mu      sync.Mutex
	players map[uint64]*Player
	nextID  uint64
	running bool
}

func (h *Hub) Serve(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", 405)
		return
	}
	// Reserve a bounded slot before the upgrade. The WebSocket library checks Origin.
	h.mu.Lock()
	if len(h.players) >= MaxPlayers {
		h.mu.Unlock()
		http.Error(w, "arena is full", 503)
		return
	}
	for _, p := range h.players {
		if p.Name == name {
			h.mu.Unlock()
			http.Error(w, "already playing in another tab", 409)
			return
		}
	}
	if h.players == nil {
		h.players = make(map[uint64]*Player)
	}
	h.nextID++
	p := &Player{ID: h.nextID, Name: name, out: make(chan []byte, 1), lastInput: time.Now()}
	h.players[p.ID] = p
	h.spawn(p, time.Now())
	// Copy welcome data before starting the simulation.
	welcome := map[string]any{"type": "welcome", "id": p.ID, "map": level, "maxPlayers": MaxPlayers, "angle": p.Angle}
	h.mu.Unlock()
	defer func() { h.mu.Lock(); delete(h.players, p.ID); h.mu.Unlock() }()
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(256)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	send, cancelSend := context.WithTimeout(ctx, time.Second)
	err = wsjson.Write(send, conn, welcome)
	cancelSend()
	if err != nil {
		return
	}
	h.mu.Lock()
	if !h.running {
		h.running = true
		go h.run()
	}
	h.mu.Unlock()
	go func() {
		defer cancel()
		defer conn.CloseNow()
		for {
			select {
			case <-ctx.Done():
				return
			case data := <-p.out:
				send, stop := context.WithTimeout(ctx, time.Second)
				err := conn.Write(send, websocket.MessageText, data)
				stop()
				if err != nil {
					return
				}
			}
		}
	}()
	window, count := time.Now(), 0
	for {
		read, stop := context.WithTimeout(ctx, 15*time.Second)
		var input Input
		err := wsjson.Read(read, conn, &input)
		stop()
		if err != nil {
			return
		}
		now := time.Now()
		if now.Sub(window) >= time.Second {
			window, count = now, 0
		}
		count++
		if count > 60 || !input.valid() {
			conn.Close(websocket.StatusPolicyViolation, "invalid or excessive input")
			return
		}
		h.mu.Lock()
		p.input = input
		p.lastInput = now
		h.mu.Unlock()
	}
}
func (h *Hub) run() {
	ticker := time.NewTicker(step)
	defer ticker.Stop()
	tick := 0
	for now := range ticker.C {
		h.mu.Lock()
		if len(h.players) == 0 {
			h.running = false
			h.mu.Unlock()
			return
		}
		h.advance(now)
		tick++
		if tick%2 == 0 {
			data, _ := json.Marshal(h.snapshot(now))
			for _, p := range h.players {
				// One most-recent snapshot, never a backlog for a slow browser.
				select {
				case <-p.out:
				default:
				}
				select {
				case p.out <- data:
				default:
				}
			}
		}
		h.mu.Unlock()
	}
}
