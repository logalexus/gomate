package analyze

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf16"

	"gomate/internal/model"
)

type compiled struct {
	p       model.Pattern
	re      *regexp.Regexp
	literal []byte
}
type snapshot struct {
	tail     int
	services map[int]model.Service
	patterns []compiled
	flags    []*regexp.Regexp
}
type Catalog struct {
	value    atomic.Pointer[snapshot]
	mu       sync.Mutex
	portHook func([]int) error
}

func NewCatalog() *Catalog {
	c := &Catalog{}
	c.value.Store(&snapshot{services: map[int]model.Service{}})
	return c
}
func (c *Catalog) Service(port int) (model.Service, bool) {
	s, ok := c.value.Load().services[port]
	return s, ok
}

func Validate(p model.Pattern) error {
	if len(p.Group) > 64 {
		return fmt.Errorf("group must not exceed 64 bytes")
	}
	if len(p.Name) == 0 || len(p.Name) > 256 || len(p.Value) == 0 || len(p.Value) > 4096 {
		return fmt.Errorf("name and non-empty value required (max 256/4096 bytes)")
	}
	if p.Direction != "INPUT" && p.Direction != "OUTPUT" && p.Direction != "BOTH" {
		return fmt.Errorf("invalid directionType")
	}
	if p.Action != "FIND" && p.Action != "IGNORE" {
		return fmt.Errorf("invalid actionType")
	}
	if p.ServiceID != nil && (*p.ServiceID < 1 || *p.ServiceID > 65535) {
		return fmt.Errorf("invalid serviceId")
	}
	if !regexp.MustCompile(`^#[0-9a-fA-F]{6}$`).MatchString(p.Color) {
		return fmt.Errorf("color must be #RRGGBB")
	}
	switch p.SearchType {
	case "REGEX":
		if _, e := regexp.Compile(p.Value); e != nil {
			return fmt.Errorf("unsupported Go/RE2 expression: %w", e)
		}
	case "SUBBYTES":
		if b, e := hex.DecodeString(p.Value); e != nil || len(b) == 0 {
			return fmt.Errorf("SUBBYTES requires non-empty hex")
		}
	case "SUBSTRING":
	default:
		return fmt.Errorf("invalid searchType")
	}
	return nil
}

func (c *Catalog) Replace(services []model.Service, patterns []model.Pattern) error {
	next := &snapshot{services: make(map[int]model.Service)}
	for _, s := range services {
		next.services[s.Port] = s
	}
	sort.Slice(patterns, func(i, j int) bool { return patterns[i].ID < patterns[j].ID })
	for _, p := range patterns {
		if !p.Enabled || p.Deleted {
			continue
		}
		if e := Validate(p); e != nil {
			return e
		}
		x := compiled{p: p}
		switch p.SearchType {
		case "REGEX":
			x.re = regexp.MustCompile(p.Value)
		case "SUBBYTES":
			x.literal, _ = hex.DecodeString(p.Value)
		default:
			x.literal = []byte(p.Value)
		}
		if x.re != nil {
			next.tail = TailBytes
		} else if len(x.literal)-1 > next.tail {
			next.tail = len(x.literal) - 1
		}
		next.patterns = append(next.patterns, x)
		if isFlag(p) {
			if x.re != nil {
				next.flags = append(next.flags, x.re)
			} else if p.SearchType == "SUBSTRING" {
				next.flags = append(next.flags, regexp.MustCompile(regexp.QuoteMeta(p.Value)))
			}
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.portHook != nil {
		ports := make([]int, 0, len(next.services))
		for port := range next.services {
			ports = append(ports, port)
		}
		sort.Ints(ports)
		if e := c.portHook(ports); e != nil {
			return e
		}
	}
	c.value.Store(next)
	return nil
}

// A FIND pattern whose name or group mentions a flag defines the flag format
// for anomaly provenance tracking.
func isFlag(p model.Pattern) bool {
	s := strings.ToLower(p.Name + " " + p.Group)
	return p.Action == "FIND" && (strings.Contains(s, "flag") || strings.Contains(s, "флаг"))
}

func (c *Catalog) FlagMatchers() []*regexp.Regexp { return c.value.Load().flags }

// Matches belong to the displayed UTF-16 string for text patterns and to raw
// bytes for SUBBYTES. Zero-length regexp matches are deliberately not highlighted.
func (c *Catalog) Analyze(chunk *model.Chunk) {
	analyzeSnapshot(chunk, c.value.Load())
}
func analyzeSnapshot(chunk *model.Chunk, snap *snapshot) {
	ids := map[int]bool{}
	for i := range chunk.Packets {
		p := &chunk.Packets[i]
		p.Matches = []model.Match{}
		text := strings.ToValidUTF8(string(p.Content), "\uFFFD")
		for _, x := range snap.patterns {
			if x.p.ServiceID != nil && *x.p.ServiceID != chunk.Stream.Service {
				continue
			}
			if x.p.Direction == "INPUT" && !p.Incoming || x.p.Direction == "OUTPUT" && p.Incoming {
				continue
			}
			var positions [][]int
			input := p.Content
			if x.p.SearchType != "SUBBYTES" {
				input = []byte(text)
			}
			if x.re != nil {
				positions = x.re.FindAllIndex(input, 256)
			} else {
				for off := 0; len(positions) < 256 && off < len(input); {
					k := bytes.Index(input[off:], x.literal)
					if k < 0 {
						break
					}
					start := off + k
					end := start + len(x.literal)
					positions = append(positions, []int{start, end})
					off = end
				}
			}
			for _, pos := range positions {
				if pos[0] == pos[1] {
					continue
				}
				if x.p.Action == "IGNORE" {
					chunk.Ignore = true
					return
				}
				if len(p.Matches) >= 1024 {
					p.Incomplete = true
					break
				}
				start, end := pos[0], pos[1]
				if x.p.SearchType != "SUBBYTES" {
					start = utf16len(string(input[:start]))
					end = utf16len(string(input[:end]))
				}
				p.Matches = append(p.Matches, model.Match{PatternID: x.p.ID, Start: start, End: end - 1})
				ids[x.p.ID] = true
			}
		}
	}
	chunk.Stream.Patterns = []int{}
	for id := range ids {
		chunk.Stream.Patterns = append(chunk.Stream.Patterns, id)
	}
	sort.Ints(chunk.Stream.Patterns)
}
func utf16len(s string) int { return len(utf16.Encode([]rune(s))) }

func (c *Catalog) SetPortHook(h func([]int) error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if h != nil {
		ports := []int{}
		for port := range c.value.Load().services {
			ports = append(ports, port)
		}
		sort.Ints(ports)
		if e := h(ports); e != nil {
			return e
		}
	}
	c.portHook = h
	return nil
}
