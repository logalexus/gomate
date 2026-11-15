// Package arena implements an ephemeral, bounded multiplayer room, independent of capture and storage.
package arena

import (
	"math"
	"sort"
	"time"
)

const MaxPlayers = 16
const step = 50 * time.Millisecond
const radius = .23

// The same map is sent in the welcome packet; the browser never defines collisions.
var level = []string{
	"1111111111111111",
	"1000000000000001",
	"1000000000000001",
	"1002200000033001",
	"1002200000033001",
	"1000000000000001",
	"1000001100000001",
	"1000001100000001",
	"1000000000000001",
	"1003300000022001",
	"1003300000022001",
	"1000000000000001",
	"1000000000000001",
	"1111111111111111",
}
var spawns = [][3]float64{{2.5, 2.5, 0}, {13.5, 11.5, math.Pi}, {13.5, 2.5, math.Pi}, {2.5, 11.5, 0}, {7.5, 2.5, math.Pi / 2}, {8.5, 11.5, -math.Pi / 2}, {2.5, 7.5, 0}, {13.5, 7.5, math.Pi}}

type Input struct {
	Forward float64 `json:"forward"`
	Strafe  float64 `json:"strafe"`
	Angle   float64 `json:"angle"`
	Pitch   float64 `json:"pitch"`
	Fire    bool    `json:"fire"`
}

func (i Input) valid() bool {
	return !math.IsNaN(i.Angle) && !math.IsInf(i.Angle, 0) && math.Abs(i.Angle) < 1e6 &&
		!math.IsNaN(i.Pitch) && math.Abs(i.Pitch) <= 1.3 &&
		!math.IsNaN(i.Forward) && math.Abs(i.Forward) <= 1 && !math.IsNaN(i.Strafe) && math.Abs(i.Strafe) <= 1
}

type Player struct {
	ID          uint64  `json:"id"`
	Name        string  `json:"name"`
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	Angle       float64 `json:"angle"`
	Pitch       float64 `json:"pitch"`
	HP          int     `json:"hp"`
	Kills       int     `json:"kills"`
	Deaths      int     `json:"deaths"`
	Shot        uint64  `json:"shot"`
	Hits        uint64  `json:"hits"`
	RespawnAt   int64   `json:"respawnAt"`
	ShieldUntil int64   `json:"shieldUntil"`
	input       Input
	lastInput   time.Time
	nextShot    time.Time
	out         chan []byte
}
type State struct {
	Type    string   `json:"type"`
	Time    int64    `json:"time"`
	Players []Player `json:"players"`
}

func wall(x, y float64) bool {
	ix, iy := int(math.Floor(x)), int(math.Floor(y))
	return iy < 0 || iy >= len(level) || ix < 0 || ix >= len(level[0]) || level[iy][ix] != '0'
}
func free(x, y float64) bool {
	return !wall(x-radius, y-radius) && !wall(x+radius, y-radius) && !wall(x-radius, y+radius) && !wall(x+radius, y+radius)
}
func wallDistance(x, y, a float64) float64 {
	// At most 800 samples per shot, with a maximum of 16 players firing every 300ms.
	for d := 0.; d < 32; d += .04 {
		if wall(x+math.Cos(a)*d, y+math.Sin(a)*d) {
			return d
		}
	}
	return 32
}
func (h *Hub) spawn(p *Player, now time.Time) {
	// Prefer a spawn far from living opponents; ties rotate between joins/deaths.
	best, score := 0, -1.
	for n := range spawns {
		i := (n + int(p.ID) + p.Deaths) % len(spawns)
		s := spawns[i]
		distance := 1000.
		for _, q := range h.players {
			if q.ID != p.ID && q.HP > 0 {
				distance = math.Min(distance, math.Hypot(s[0]-q.X, s[1]-q.Y))
			}
		}
		if distance > score {
			best, score = i, distance
		}
	}
	s := spawns[best]
	p.X, p.Y, p.Angle = s[0], s[1], s[2]
	p.Pitch = 0
	p.HP = 100
	p.RespawnAt = 0
	p.ShieldUntil = now.Add(time.Second).UnixMilli()
	p.input = Input{Angle: p.Angle}
}
func (h *Hub) advance(now time.Time) {
	ids := make([]uint64, 0, len(h.players))
	for id := range h.players {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		p := h.players[id]
		if p.HP <= 0 {
			if now.UnixMilli() >= p.RespawnAt {
				h.spawn(p, now)
			}
			continue
		}
		if now.Sub(p.lastInput) > 250*time.Millisecond {
			continue
		}
		p.Angle = math.Remainder(p.input.Angle, 2*math.Pi)
		p.Pitch = p.input.Pitch
		f, s := p.input.Forward, p.input.Strafe
		if length := math.Hypot(f, s); length > 1 {
			f /= length
			s /= length
		}
		speed := 3.5 * step.Seconds()
		dx := (math.Cos(p.Angle)*f - math.Sin(p.Angle)*s) * speed
		dy := (math.Sin(p.Angle)*f + math.Cos(p.Angle)*s) * speed
		if free(p.X+dx, p.Y) {
			p.X += dx
		}
		if free(p.X, p.Y+dy) {
			p.Y += dy
		}
	}
	for _, id := range ids {
		p := h.players[id]
		if p.HP <= 0 || !p.input.Fire || now.Sub(p.lastInput) > 250*time.Millisecond || now.Before(p.nextShot) {
			continue
		}
		p.nextShot = now.Add(300 * time.Millisecond)
		p.Shot++
		p.ShieldUntil = 0
		closest := wallDistance(p.X, p.Y, p.Angle)
		var victim *Player
		for _, qid := range ids {
			q := h.players[qid]
			if q.ID == p.ID || q.HP <= 0 || q.ShieldUntil > now.UnixMilli() {
				continue
			}
			dx, dy := q.X-p.X, q.Y-p.Y
			along := dx*math.Cos(p.Angle) + dy*math.Sin(p.Angle)
			across := math.Abs(-dx*math.Sin(p.Angle) + dy*math.Cos(p.Angle))
			if along > 0 && across < .32 {
				root := math.Sqrt(.32*.32 - across*across)
				hit, end := math.Max(0, along-root), along+root
				// Intersect the horizontal circle with the player's vertical cylinder.
				// Eye height is 1.25m, target height 1.6m; looking over a head must miss.
				if slope := math.Tan(p.Pitch); math.Abs(slope) > 1e-8 {
					lo, hi := -1.25/slope, (1.6-1.25)/slope
					hit = math.Max(hit, math.Min(lo, hi))
					end = math.Min(end, math.Max(lo, hi))
				}
				if hit <= end && hit < closest {
					closest = hit
					victim = q
				}
			}
		}
		if victim != nil {
			p.Hits++
			victim.HP = max(0, victim.HP-34)
			if victim.HP == 0 {
				p.Kills++
				victim.Deaths++
				victim.RespawnAt = now.Add(3 * time.Second).UnixMilli()
				victim.input = Input{}
			}
		}
	}
}
func (h *Hub) snapshot(now time.Time) State {
	state := State{Type: "state", Time: now.UnixMilli(), Players: make([]Player, 0, len(h.players))}
	for _, p := range h.players {
		state.Players = append(state.Players, *p)
	}
	sort.Slice(state.Players, func(i, j int) bool { return state.Players[i].ID < state.Players[j].ID })
	return state
}
