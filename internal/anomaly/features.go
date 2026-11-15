package anomaly

// Total value features: every byte sequence has a group mask, a shape and a
// length bucket. Nothing here classifies values into semantic types.
import (
	"bytes"
	"math/bits"
	"strconv"
	"strings"
	"unicode/utf8"
)

const groupDots = 31

var groupOf [256]uint8

var groupNames = [32]string{"a-z", "A-Z", "0-9", "space", "'", "\"", "`", "< >", "{ }", "( )", "[ ]", ";", "| &", "$", "\\", "/", ".", "%", "#", "=", ",", ":", "*", "! ?", "@", "~ ^", "+", "- _", "\\t \\r \\n", "control bytes", "non-ASCII", ".."}

func init() {
	for i := range groupOf {
		c := byte(i)
		switch {
		case c >= 'a' && c <= 'z':
			groupOf[i] = 0
		case c >= 'A' && c <= 'Z':
			groupOf[i] = 1
		case c >= '0' && c <= '9':
			groupOf[i] = 2
		case c == '\t' || c == '\r' || c == '\n':
			groupOf[i] = 28
		case c < 0x20 || c == 0x7f:
			groupOf[i] = 29
		case c >= 0x80:
			groupOf[i] = 30
		}
	}
	for g, chars := range map[uint8]string{3: " ", 4: "'", 5: "\"", 6: "`", 7: "<>", 8: "{}", 9: "()", 10: "[]", 11: ";", 12: "|&", 13: "$", 14: "\\", 15: "/", 16: ".", 17: "%", 18: "#", 19: "=", 20: ",", 21: ":", 22: "*", 23: "!?", 24: "@", 25: "~^", 26: "+", 27: "-_"} {
		for i := 0; i < len(chars); i++ {
			groupOf[chars[i]] = g
		}
	}
}

func maskOf(b []byte) uint32 {
	var m uint32
	for _, c := range b {
		m |= 1 << groupOf[c]
	}
	if bytes.Contains(b, []byte("..")) {
		m |= 1 << groupDots
	}
	return m
}

func describeMask(m uint32) string {
	parts := []string{}
	for g := 0; g < 32; g++ {
		if m&(1<<g) != 0 {
			parts = append(parts, groupNames[g])
		}
	}
	return strings.Join(parts, "  ")
}

// shapeOf collapses alphanumeric runs to "w" and keeps punctuation, so
// "user_123" and "Xa91kLm2pQ" are both stable while "x' OR '1'='1" is not.
func shapeOf(b []byte) string {
	var s strings.Builder
	last := byte(0)
	for _, c := range b {
		var x byte
		switch g := groupOf[c]; {
		case g <= 2:
			x = 'w'
		case g == 28 || g == 29:
			x = 'c'
		case g == 30:
			x = 'u'
		default:
			x = c
		}
		if x == last && (x == 'w' || x == 'c' || x == 'u') {
			continue
		}
		if s.Len() >= 16 {
			s.WriteString("…")
			break
		}
		s.WriteByte(x)
		last = x
	}
	return s.String()
}

const lenBuckets = 40

// Half-octave buckets: 0, 1, 2, 3, 4-5, 6-7, 8-11, 12-15, ...
func lenBucket(n int) int {
	if n <= 0 {
		return 0
	}
	l := bits.Len(uint(n))
	b := 2 * l
	if l >= 2 && n&(1<<(l-2)) == 0 {
		b--
	}
	b--
	if b >= lenBuckets {
		b = lenBuckets - 1
	}
	return b
}

func bucketRange(b int) (int, int) {
	if b < 2 {
		return b, b
	}
	if b%2 == 0 {
		l := b/2 + 1
		lo := 1 << (l - 1)
		return lo, lo + 1<<(l-2) - 1
	}
	l := (b + 1) / 2
	return 1<<(l-1) + 1<<(l-2), 1<<l - 1
}

func hash64(b []byte) uint64 {
	h := uint64(14695981039346656037)
	for _, c := range b {
		h ^= uint64(c)
		h *= 1099511628211
	}
	return h
}

// preview renders a value for an explanation line without control bytes.
func preview(b []byte, limit int) string {
	var s strings.Builder
	for len(b) > 0 && s.Len() < limit {
		r, n := utf8.DecodeRune(b)
		if r == utf8.RuneError && n == 1 || r < 0x20 || r == 0x7f {
			s.WriteString(`\x` + strconv.FormatUint(uint64(b[0]), 16))
		} else {
			s.WriteRune(r)
		}
		b = b[n:]
	}
	if len(b) > 0 {
		s.WriteString("…")
	}
	return s.String()
}
