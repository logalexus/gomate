package arena

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func fixture(now time.Time) (*Hub, *Player, *Player) {
	a := &Player{ID: 1, Name: "a", X: 2, Y: 2, HP: 100, lastInput: now}
	b := &Player{ID: 2, Name: "b", X: 5, Y: 2, HP: 100, lastInput: now}
	return &Hub{players: map[uint64]*Player{1: a, 2: b}}, a, b
}
func TestCombatWallsCooldownRespawn(t *testing.T) {
	now := time.Now()
	h, a, b := fixture(now)
	a.input = Input{Fire: true}
	h.advance(now)
	if b.HP != 66 || a.Hits != 1 {
		t.Fatalf("hit: %+v %+v", a, b)
	}
	h.advance(now.Add(step))
	if b.HP != 66 {
		t.Fatal("cooldown ignored")
	}
	for i := 1; i <= 2; i++ {
		at := now.Add(time.Duration(i) * 300 * time.Millisecond)
		a.lastInput = at
		h.advance(at)
	}
	if b.HP != 0 || a.Kills != 1 || b.Deaths != 1 {
		t.Fatalf("kill: %+v %+v", a, b)
	}
	h.advance(time.UnixMilli(b.RespawnAt))
	if b.HP != 100 || !free(b.X, b.Y) || b.ShieldUntil == 0 {
		t.Fatal("respawn failed")
	}
	h, a, b = fixture(now)
	a.X, a.Y, b.X, b.Y = 2, 3.5, 6, 3.5
	a.input = Input{Fire: true}
	h.advance(now)
	if b.HP != 100 {
		t.Fatal("shot through cover")
	}
	h, a, b = fixture(now)
	b.ShieldUntil = now.Add(time.Second).UnixMilli()
	a.input = Input{Fire: true}
	h.advance(now)
	if b.HP != 100 {
		t.Fatal("spawn protection ignored")
	}
}
func TestMovementBoundedAndStaleInputStops(t *testing.T) {
	now := time.Now()
	h, a, _ := fixture(now)
	a.input = Input{Forward: 1, Strafe: 1}
	x, y := a.X, a.Y
	h.advance(now)
	if math.Abs(math.Hypot(a.X-x, a.Y-y)-3.5*step.Seconds()) > 1e-8 {
		t.Fatal("diagonal speed advantage")
	}
	x, y = a.X, a.Y
	h.advance(now.Add(time.Second))
	if a.X != x || a.Y != y {
		t.Fatal("stale input kept moving")
	}
	a.X, a.Y = 1.3, 1.3
	a.input = Input{Forward: 1, Angle: math.Pi}
	for i := 0; i < 100; i++ {
		h.advance(now)
	}
	if !free(a.X, a.Y) || a.X < 1.23 {
		t.Fatal("walked through wall")
	}
	for _, input := range []Input{{Forward: 2}, {Strafe: math.Inf(1)}, {Angle: math.NaN()}, {Angle: math.Inf(1)}} {
		if input.valid() {
			t.Fatal("invalid input accepted")
		}
	}
}
func TestNearestPlayerAndDeadPlayer(t *testing.T) {
	now := time.Now()
	h, a, b := fixture(now)
	c := &Player{ID: 3, X: 7, Y: 2, HP: 100}
	h.players[3] = c
	a.input = Input{Fire: true}
	h.advance(now)
	if b.HP != 66 || c.HP != 100 {
		t.Fatal("hitscan did not stop at first target")
	}
	a.HP = 0
	a.RespawnAt = now.Add(time.Second).UnixMilli()
	a.lastInput = now.Add(300 * time.Millisecond)
	h.advance(a.lastInput)
	if b.HP != 66 {
		t.Fatal("dead player fired")
	}
}
func TestWebSocketRoomBoundedAndStops(t *testing.T) {
	var h Hub
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { h.Serve(w, r, r.URL.Query().Get("name")) }))
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	url := "ws" + strings.TrimPrefix(s.URL, "http")
	clients := []*websocket.Conn{}
	defer func() {
		for _, c := range clients {
			c.CloseNow()
		}
	}()
	for i := 0; i < MaxPlayers; i++ {
		c, _, err := websocket.Dial(ctx, url+fmt.Sprintf("?name=p%d", i), nil)
		if err != nil {
			t.Fatal(err)
		}
		clients = append(clients, c)
		var welcome struct {
			Type string
			ID   uint64
			Map  []string
		}
		if err = wsjson.Read(ctx, c, &welcome); err != nil {
			t.Fatal(err)
		}
		if welcome.Type != "welcome" || len(welcome.Map) == 0 {
			t.Fatal("bad welcome")
		}
	}
	c, response, err := websocket.Dial(ctx, url+"?name=extra", nil)
	if c != nil {
		c.CloseNow()
	}
	if err == nil || response.StatusCode != 503 {
		t.Fatal("room cap failed")
	}
	var state State
	for len(state.Players) != MaxPlayers {
		if err = wsjson.Read(ctx, clients[0], &state); err != nil {
			t.Fatal(err)
		}
	}
	clients[1].CloseNow()
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		h.mu.Lock()
		n := len(h.players)
		h.mu.Unlock()
		if n == 15 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	c, response, err = websocket.Dial(ctx, url+"?name=p0", nil)
	if c != nil {
		c.CloseNow()
	}
	if err == nil || response.StatusCode != 409 {
		t.Fatal("duplicate browser accepted")
	}
	for _, c := range clients {
		c.CloseNow()
	}
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		h.mu.Lock()
		done := len(h.players) == 0 && !h.running
		h.mu.Unlock()
		if done {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("empty room kept ticking")
}
func TestWebSocketRejectsOriginAndBadInput(t *testing.T) {
	var h Hub
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { h.Serve(w, r, "p") }))
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := "ws" + strings.TrimPrefix(s.URL, "http")
	c, resp, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{"https://foreign.example"}}})
	if c != nil {
		c.CloseNow()
	}
	if err == nil || resp.StatusCode != 403 {
		t.Fatal("foreign origin allowed")
	}
	c, _, err = websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	var welcome any
	if err = wsjson.Read(ctx, c, &welcome); err != nil {
		t.Fatal(err)
	}
	if err = wsjson.Write(ctx, c, Input{Forward: 500}); err != nil {
		t.Fatal(err)
	}
	for {
		_, _, err = c.Read(ctx)
		if err != nil {
			break
		}
	}
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Fatalf("bad input status: %v", err)
	}
}
func BenchmarkFullRoomTick(b *testing.B) {
	now := time.Now()
	h := &Hub{players: map[uint64]*Player{}}
	for i := 1; i <= MaxPlayers; i++ {
		p := &Player{ID: uint64(i)}
		h.players[p.ID] = p
		h.spawn(p, now)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		now = now.Add(step)
		for _, p := range h.players {
			p.input = Input{Forward: 1, Fire: true, Angle: p.Angle + .1}
			p.lastInput = now
		}
		h.advance(now)
		if i%2 == 0 {
			json.Marshal(h.snapshot(now))
		}
	}
}

func TestInputFloodDisconnected(t *testing.T) {
	var h Hub
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { h.Serve(w, r, "flood") }))
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(s.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	var welcome any
	if err = wsjson.Read(ctx, c, &welcome); err != nil {
		t.Fatal(err)
	}
	go func() {
		for i := 0; i < 100; i++ {
			if wsjson.Write(ctx, c, Input{}) != nil {
				return
			}
		}
	}()
	for {
		_, _, err = c.Read(ctx)
		if err != nil {
			break
		}
	}
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Fatalf("flood not rejected: %v", err)
	}
}
