package dockerui

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const maxLogRead = 2 << 20
const maxLogText = 512 << 10
const maxLine = 4096

var ansi = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)

type LogLine struct {
	ID     string `json:"id"`
	Time   string `json:"time"`
	Stream string `json:"stream"`
	Text   string `json:"text"`
}
type LogSnapshot struct {
	Lines     []LogLine `json:"lines"`
	Truncated bool      `json:"truncated"`
	TTY       bool      `json:"tty"`
	FetchedAt time.Time `json:"fetchedAt"`
}

// Docker logs are a bounded snapshot. TTY output is raw; otherwise an 8-byte
// header precedes each stdout/stderr frame, which may split a logical line.
func ReadLogs(r io.Reader, tty bool, tail int) (LogSnapshot, error) {
	out := LogSnapshot{Lines: []LogLine{}, TTY: tty, FetchedAt: time.Now()}
	if tail < 1 || tail > 2000 {
		return out, &Error{400, "tail must be between 1 and 2000"}
	}
	b, err := io.ReadAll(io.LimitReader(r, maxLogRead+1))
	if err != nil {
		return out, err
	}
	if len(b) > maxLogRead {
		out.Truncated = true
		b = b[:maxLogRead]
	}
	pending := map[string][]byte{}
	clipped := map[string]bool{}
	total := 0
	sequence := 0
	emit := func(stream string) {
		line := string(bytes.ToValidUTF8(pending[stream], []byte("�")))
		line = ansi.ReplaceAllString(line, "")
		line = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) && r != '\t' {
				return -1
			}
			return r
		}, line)
		if clipped[stream] {
			line += " … [line truncated]"
			out.Truncated = true
		}
		stamp := ""
		if space := strings.IndexByte(line, ' '); space > 0 {
			if _, e := time.Parse(time.RFC3339Nano, line[:space]); e == nil {
				stamp = line[:space]
				line = line[space+1:]
			}
		}
		digest := sha256.Sum256([]byte(fmt.Sprintf("%s/%s/%s/%d", stream, stamp, line, sequence)))
		sequence++
		out.Lines = append(out.Lines, LogLine{ID: fmt.Sprintf("%x", digest[:12]), Time: stamp, Stream: stream, Text: line})
		total += len(line)
		pending[stream] = nil
		clipped[stream] = false
		for len(out.Lines) > tail || total > maxLogText {
			total -= len(out.Lines[0].Text)
			out.Lines = out.Lines[1:]
			out.Truncated = true
		}
	}
	feed := func(stream string, payload []byte) {
		for len(payload) > 0 {
			i := bytes.IndexByte(payload, '\n')
			part := payload
			if i >= 0 {
				part = payload[:i]
			}
			room := maxLine - len(pending[stream])
			if len(part) > room {
				clipped[stream] = true
			}
			pending[stream] = append(pending[stream], part[:min(len(part), room)]...)
			if i < 0 {
				break
			}
			emit(stream)
			payload = payload[i+1:]
		}
	}
	if tty {
		feed("tty", b)
	} else {
		for len(b) > 0 {
			if len(b) < 8 {
				out.Truncated = true
				break
			}
			if (b[0] != 1 && b[0] != 2) || b[1] != 0 || b[2] != 0 || b[3] != 0 {
				return out, &Error{502, "Unknown Docker log format"}
			}
			size := int64(binary.BigEndian.Uint32(b[4:8]))
			stream := "stdout"
			if b[0] == 2 {
				stream = "stderr"
			}
			b = b[8:]
			n := min(int64(len(b)), size)
			feed(stream, b[:n])
			b = b[n:]
			if n < size {
				out.Truncated = true
				break
			}
		}
	}
	for _, stream := range []string{"stdout", "stderr", "tty"} {
		if len(pending[stream]) > 0 || clipped[stream] {
			emit(stream)
		}
	}
	return out, nil
}
