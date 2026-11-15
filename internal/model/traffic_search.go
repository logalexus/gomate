package model

import (
	"fmt"
	"regexp"
)

// SearchCursor resumes inside a large session without rescanning its packets.
type SearchCursor struct {
	Before      *int64 `json:"before"`
	InStream    int64  `json:"inStream"`
	AfterPacket int64  `json:"afterPacket"`
}

type TrafficSearch struct {
	Page
	Query  string        `json:"query"`
	Mode   string        `json:"mode"`
	Port   int           `json:"port"`
	Cursor *SearchCursor `json:"cursor"`
}

type TrafficSearchResult struct {
	Streams []Stream      `json:"streams"`
	Next    *SearchCursor `json:"next"`
	Scanned int           `json:"scanned"`
}

// Compile uses RE2, with linear matching time even for nested repetitions.
// Text searches treat all punctuation literally. Both modes ignore case by default.
func (s TrafficSearch) Compile() (*regexp.Regexp, error) {
	if len(s.Query) == 0 || len(s.Query) > 512 {
		return nil, fmt.Errorf("search must contain 1–512 bytes")
	}
	if s.Port < 0 || s.Port > 65535 {
		return nil, fmt.Errorf("invalid service port")
	}
	if c := s.Cursor; c != nil {
		if c.Before != nil && *c.Before < 1 || c.InStream < 0 || c.AfterPacket < 0 ||
			(c.InStream != 0 && (c.Before == nil || c.InStream != *c.Before-1)) ||
			(c.InStream == 0 && c.AfterPacket != 0) {
			return nil, fmt.Errorf("invalid search cursor")
		}
	}
	value := s.Query
	switch s.Mode {
	case "", "text":
		value = regexp.QuoteMeta(value)
	case "regex":
	default:
		return nil, fmt.Errorf("search mode must be text or regex")
	}
	r, err := regexp.Compile("(?i)" + value)
	if err != nil {
		return nil, fmt.Errorf("invalid regular expression: %v", err)
	}
	return r, nil
}
