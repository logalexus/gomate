package anomaly

// Per-service model of normal traffic. Every observation is first scored
// against the counters, then learned, so the first unusual request is reported.
import (
	"fmt"
	"math"
	"math/bits"
	"sort"
	"strconv"
	"strings"
)

const (
	warmService  = 100 // messages before "new endpoint" is reported
	minTemplate  = 30  // template observations before field checks
	minField     = 20  // field observations before value checks
	pathFanout   = 10  // distinct children before a path position becomes {*}
	maxTemplates = 1024
	maxFields    = 128
	maxNodes     = 4096
	maxVerbs     = 64
	enumValues   = 32
	trackValues  = 64
	maxShapes    = 16
	reasonCap    = 16.0
	rareShare    = 200 // a feature seen in fewer than 1/rareShare observations is rare
	rareCount    = 10  // ... or fewer than rareCount times
)

// rare keeps repeated attacks visible until they are a real share of traffic.
func rare(count, total uint32) bool {
	return count < rareCount || uint64(count)*rareShare < uint64(total)
}

type Reason struct {
	Bits     float64 `json:"bits"`
	Detector string  `json:"detector"`
	Template string  `json:"template,omitempty"`
	Field    string  `json:"field,omitempty"`
	Text     string  `json:"text"`
	Mask     uint32  `json:"mask,omitempty"`
	Hash     string  `json:"hash,omitempty"`
	Example  string  `json:"example,omitempty"`
	Shape    string  `json:"shape,omitempty"`
	Length   int     `json:"length,omitempty"`
}

type fieldModel struct {
	n          uint32
	groups     [32]uint32
	values     map[uint64]uint32 // nil once more than trackValues distinct values were seen
	examples   map[uint64]string
	lens       [lenBuckets]uint32
	shapes     map[string]uint32
	shapeOther uint32
}

type templateModel struct {
	n      uint32
	fields map[string]*fieldModel
}

type pathNode struct {
	children map[string]*pathNode
	wild     *pathNode
}

type serviceModel struct {
	n         uint64
	templates map[string]*templateModel
	roots     map[string]*pathNode
	nodes     int
	verbs     map[string]bool
	stats     ServiceStats
}

func newService() *serviceModel {
	return &serviceModel{templates: map[string]*templateModel{}, roots: map[string]*pathNode{}, verbs: map[string]bool{}}
}

// templatize maps a request path onto the learned path tree. A position with
// more than pathFanout distinct values becomes {*}; its values become fields.
func (s *serviceModel) templatize(m *message) {
	if m.path == "" {
		return
	}
	root := s.roots[m.method]
	if root == nil {
		if len(s.roots) >= 32 {
			m.tpl = m.method + " (other method)"
			return
		}
		root = &pathNode{children: map[string]*pathNode{}}
		s.roots[m.method] = root
	}
	segs := strings.Split(strings.Trim(m.path, "/"), "/")
	out := make([]string, 0, len(segs))
	n := root
	for i, seg := range segs {
		if i >= 12 {
			out = append(out, "…")
			break
		}
		if n.wild != nil {
			out = append(out, "{*}")
			m.add("path["+strconv.Itoa(i)+"]", []byte(unescape(seg)), kindValue, linkWeak)
			n = n.wild
			continue
		}
		if c := n.children[seg]; c != nil {
			out = append(out, seg)
			n = c
			continue
		}
		if len(n.children) >= pathFanout || s.nodes >= maxNodes {
			n.children = nil
			n.wild = &pathNode{children: map[string]*pathNode{}}
			s.nodes++
			out = append(out, "{*}")
			m.add("path["+strconv.Itoa(i)+"]", []byte(unescape(seg)), kindValue, linkWeak)
			n = n.wild
			continue
		}
		c := &pathNode{children: map[string]*pathNode{}}
		n.children[seg] = c
		s.nodes++
		out = append(out, seg)
		n = c
	}
	m.tpl = m.method + " /" + strings.Join(out, "/")
}

func (s *serviceModel) verb(v string) string {
	if s.verbs[v] {
		return v
	}
	if len(s.verbs) >= maxVerbs {
		return "*"
	}
	s.verbs[v] = true
	return v
}

func surprise(seen, total float64) float64 {
	if total <= 0 {
		return 0
	}
	return math.Log2(total / math.Max(seen, 1))
}

func capBits(b, limit float64) float64 {
	b = math.Min(b, limit)
	return math.Round(b*10) / 10
}

// common is the set of byte groups that are not rare in this field.
func (f *fieldModel) common() uint32 {
	var m uint32
	for g, c := range f.groups {
		if c > 0 && !rare(c, f.n) {
			m |= 1 << g
		}
	}
	return m
}

// Enum detection counts only values that are not rare, so a few injected
// values do not turn an enumeration into a free-form field.
func (f *fieldModel) isEnum() bool {
	if f.values == nil {
		return false
	}
	common := 0
	var covered uint32
	for _, c := range f.values {
		if !rare(c, f.n) {
			common++
			covered += c
		}
	}
	return common > 0 && common <= enumValues && covered*10 >= f.n*9
}

func (f *fieldModel) commonExamples() string {
	parts := []string{}
	for h, x := range f.examples {
		if !rare(f.values[h], f.n) {
			parts = append(parts, x)
		}
	}
	sort.Strings(parts)
	if len(parts) > 6 {
		parts = append(parts[:6], "…")
	}
	return strings.Join(parts, ", ")
}

func (f *fieldModel) lenRange() (int, int) {
	lo, hi := -1, -1
	var acc uint32
	for b, c := range f.lens {
		acc += c
		if lo < 0 && acc*20 > f.n {
			lo, _ = bucketRange(b)
		}
		if hi < 0 && acc*20 >= f.n*19 {
			_, hi = bucketRange(b)
		}
	}
	return lo, hi
}

func (f *fieldModel) topShapes() string {
	type kv struct {
		k string
		v uint32
	}
	xs := make([]kv, 0, len(f.shapes))
	for k, v := range f.shapes {
		xs = append(xs, kv{k, v})
	}
	sort.Slice(xs, func(i, j int) bool { return xs[i].v > xs[j].v })
	parts := []string{}
	for i, x := range xs {
		if i >= 3 || rare(x.v, f.n) {
			break
		}
		parts = append(parts, "«"+x.k+"»")
	}
	return strings.Join(parts, ", ")
}

func isHeader(key string) bool { return strings.HasPrefix(key, "header.") }

// score explains how an observation differs from the learned template.
func (s *serviceModel) score(m *message) []Reason {
	out := []Reason{}
	t := s.templates[m.tpl]
	if s.n >= warmService && (t == nil || uint64(t.n)*rareShare < s.n) {
		text := fmt.Sprintf("new endpoint: not seen in %d service messages", s.n)
		var seen uint32
		if t != nil && t.n > 0 {
			seen = t.n
			text = fmt.Sprintf("rare endpoint: %d of %d service messages", t.n, s.n)
		}
		out = append(out, Reason{Bits: capBits(surprise(float64(seen)+3, float64(s.n)), 10), Detector: "template", Template: m.tpl, Text: text})
	}
	if t == nil {
		return out
	}
	if t.n < minTemplate {
		return out
	}
	present := map[string]bool{}
	for _, f := range m.fields {
		present[f.key] = true
		fm := t.fields[f.key]
		if fm == nil || rare(fm.n*(rareShare/50), t.n) {
			if fm != nil || len(t.fields) < maxFields {
				limit := 8.0
				if isHeader(f.key) {
					limit = 3
				}
				var seen uint32
				text := fmt.Sprintf("new field (not seen in %d endpoint messages)", t.n)
				if fm != nil {
					seen = fm.n
					text = fmt.Sprintf("rare field: seen in %d of %d endpoint messages", fm.n, t.n)
				}
				out = append(out, Reason{Bits: capBits(surprise(float64(seen)+3, float64(t.n)), limit), Detector: "field", Template: m.tpl, Field: f.key, Text: text})
			}
			if fm == nil {
				continue
			}
		}
		if fm.n < minField || f.kind == kindPresence {
			continue
		}
		out = append(out, fm.score(m.tpl, f)...)
	}
	// Missing fields of one message are reported once: a different response
	// format usually drops several fields together.
	missing := []string{}
	var worst float64
	for key, fm := range t.fields {
		if present[key] || fm.n*50 < t.n*49 {
			continue
		}
		limit := 6.0
		if isHeader(key) {
			limit = 3
		}
		missing = append(missing, key)
		worst = math.Max(worst, capBits(surprise(float64(t.n-fm.n)+1, float64(t.n)), limit))
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		text := fmt.Sprintf("missing field that is usually present (%d endpoint messages)", t.n)
		if len(missing) > 1 {
			text = fmt.Sprintf("missing fields that are usually present: %s", strings.Join(missing, ", "))
		}
		out = append(out, Reason{Bits: capBits(worst+math.Log2(float64(len(missing))), 8), Detector: "missing", Template: m.tpl, Field: missing[0], Text: text})
	}
	return out
}

func (f *fieldModel) score(tpl string, x field) []Reason {
	out := []Reason{}
	n := float64(f.n)
	h := hash64(x.value)
	if x.kind == kindEnum || x.kind == kindValue && f.isEnum() {
		c, ok := f.values[h]
		if ok && !rare(c, f.n) {
			return out
		}
		if f.values != nil {
			ex := preview(x.value, 48)
			text := fmt.Sprintf("new value “%s”; usually: %s", ex, f.commonExamples())
			if ok {
				text = fmt.Sprintf("rare value “%s”: %d of %d; usually: %s", ex, c, f.n, f.commonExamples())
			}
			out = append(out, Reason{Bits: capBits(surprise(float64(c)+1, n), 12), Detector: "enum", Template: tpl, Field: x.key, Text: text, Hash: strconv.FormatUint(h, 16), Example: ex})
		}
		if x.kind == kindEnum {
			return out
		}
	}
	if x.kind == kindValue {
		common := f.common()
		if extra := maskOf(x.value) &^ common; extra != 0 {
			least := f.n
			var fresh uint32
			for g := 0; g < 32; g++ {
				if extra&(1<<g) != 0 {
					least = min(least, f.groups[g])
					if f.groups[g] == 0 {
						fresh |= 1 << g
					}
				}
			}
			parts := []string{}
			if fresh != 0 {
				parts = append(parts, "first seen "+describeMask(fresh))
			}
			if seldom := extra &^ fresh; seldom != 0 {
				parts = append(parts, "rare "+describeMask(seldom))
			}
			text := fmt.Sprintf("characters: %s in “%s” (usually: %s; values: %d)", strings.Join(parts, "; "), preview(x.value, 60), describeMask(common), f.n)
			out = append(out, Reason{Bits: capBits(surprise(float64(least)+3, n)+float64(bits.OnesCount32(extra)-1), 14), Detector: "chars", Template: tpl, Field: x.key, Text: text, Mask: extra})
		}
		if f.values == nil && len(f.shapes) > 0 && f.shapeOther*10 < f.n {
			sh := shapeOf(x.value)
			if c := f.shapes[sh]; rare(c, f.n) {
				out = append(out, Reason{Bits: capBits(surprise(float64(c)+1, n), 8), Detector: "shape", Template: tpl, Field: x.key, Text: fmt.Sprintf("unusual value shape “%s”; usually %s", sh, f.topShapes()), Shape: sh})
			}
		}
	}
	b := lenBucket(x.size)
	near := float64(f.lens[b])
	if b > 0 {
		near += float64(f.lens[b-1]) / 2
	}
	if b+1 < lenBuckets {
		near += float64(f.lens[b+1]) / 2
	}
	if bitsLen := surprise(near+0.1, n+1); bitsLen > 6 {
		lo, hi := f.lenRange()
		out = append(out, Reason{Bits: capBits(bitsLen, 10), Detector: "length", Template: tpl, Field: x.key, Text: fmt.Sprintf("unusual length: %d bytes (usually %d–%d)", x.size, lo, hi), Length: x.size})
	}
	return out
}

func (s *serviceModel) template(tpl string) *templateModel {
	t := s.templates[tpl]
	if t == nil {
		if len(s.templates) >= maxTemplates {
			return nil
		}
		t = &templateModel{fields: map[string]*fieldModel{}}
		s.templates[tpl] = t
	}
	return t
}

func (s *serviceModel) learn(m *message) {
	s.n++
	t := s.template(m.tpl)
	if t == nil {
		return
	}
	t.n++
	seen := map[string]bool{}
	for _, x := range m.fields {
		if seen[x.key] {
			continue
		}
		seen[x.key] = true
		f := t.fields[x.key]
		if f == nil {
			if len(t.fields) >= maxFields {
				continue
			}
			f = &fieldModel{values: map[uint64]uint32{}, examples: map[uint64]string{}, shapes: map[string]uint32{}}
			t.fields[x.key] = f
		}
		f.learn(x, 1)
	}
}

func (f *fieldModel) learn(x field, weight uint32) {
	f.n += weight
	if x.kind == kindPresence {
		return
	}
	f.lens[lenBucket(x.size)] += weight
	if x.kind == kindLen {
		return
	}
	if f.values != nil {
		h := hash64(x.value)
		if _, ok := f.values[h]; !ok && len(f.values) >= trackValues {
			f.values = nil
			f.examples = nil
		} else {
			if !ok && len(f.examples) < enumValues {
				f.examples[h] = "«" + preview(x.value, 24) + "»"
			}
			f.values[h] += weight
		}
	}
	if x.kind == kindEnum {
		return
	}
	mask := maskOf(x.value)
	for g := 0; g < 32; g++ {
		if mask&(1<<g) != 0 {
			f.groups[g] += weight
		}
	}
	sh := shapeOf(x.value)
	if _, ok := f.shapes[sh]; ok || len(f.shapes) < maxShapes {
		f.shapes[sh] += weight
	} else {
		f.shapeOther += weight
	}
}

// accept applies operator feedback: the reported deviation becomes normal.
func (s *serviceModel) accept(r Reason) {
	t := s.templates[r.Template]
	if t == nil {
		return
	}
	f := t.fields[r.Field]
	if f == nil {
		return
	}
	weight := f.n/10 + 3
	switch r.Detector {
	case "chars":
		for g := 0; g < 32; g++ {
			if r.Mask&(1<<g) != 0 {
				f.groups[g] += weight
			}
		}
	case "enum":
		h, e := strconv.ParseUint(r.Hash, 16, 64)
		if e == nil && f.values != nil {
			if _, ok := f.values[h]; !ok && len(f.examples) < enumValues {
				f.examples[h] = "«" + r.Example + "»"
			}
			f.values[h] += weight
		}
	case "shape":
		f.shapes[r.Shape] += weight
	case "length":
		f.lens[lenBucket(r.Length)] += weight
	}
}
