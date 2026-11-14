package model

import (
	"fmt"
	"strconv"
	"time"
)

type Service struct {
	Port       int    `json:"port"`
	Name       string `json:"name"`
	HTTP       bool   `json:"http"`
	URLDecode  bool   `json:"urldecodeHttpRequests"`
	Merge      bool   `json:"mergeAdjacentPackets"`
	WebSockets bool   `json:"parseWebSockets"`
	TLS        bool   `json:"decryptTls"`
}

type Pattern struct {
	Group      string `json:"group"`
	ID         int    `json:"id"`
	Enabled    bool   `json:"enabled"`
	Deleted    bool   `json:"deleted"`
	Name       string `json:"name"`
	Value      string `json:"value"`
	Color      string `json:"color"`
	SearchType string `json:"searchType"`
	Direction  string `json:"directionType"`
	Action     string `json:"actionType"`
	ServiceID  *int   `json:"serviceId"`
}

type Match struct {
	CrossBoundary bool `json:"crossBoundary,omitempty"`
	PatternID     int  `json:"patternId"`
	Start         int  `json:"startPosition"`
	End           int  `json:"endPosition"`
}

type Attachment struct {
	Name         string `json:"name"`
	DownloadName string `json:"downloadName"`
	Field        string `json:"field,omitempty"`
	Type         string `json:"type"`
	DeclaredType string `json:"declaredType,omitempty"`
	Size         int    `json:"size"`
	Content      []byte `json:"content"`
}

type Packet struct {
	Attachments     []Attachment      `json:"attachments,omitempty"`
	Preview         string            `json:"preview,omitempty"`
	Kind            string            `json:"kind,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	ID              int64             `json:"id"`
	Timestamp       int64             `json:"timestamp"`
	Incoming        bool              `json:"incoming"`
	Content         []byte            `json:"content"`
	Matches         []Match           `json:"matches"`
	Ungzipped       bool              `json:"ungzipped"`
	WebSocketParsed bool              `json:"webSocketParsed"`
	TLSDecrypted    bool              `json:"tlsDecrypted"`
	HasHTTPBody     bool              `json:"hasHttpBody"`
	Incomplete      bool              `json:"incomplete"`
}

type Stream struct {
	FirstRequestPath      string  `json:"firstRequestPath"`
	FirstRequestMethod    string  `json:"firstRequestMethod,omitempty"`
	FirstRequestTruncated bool    `json:"firstRequestTruncated,omitempty"`
	PayloadPurged         bool    `json:"payloadPurged"`
	ID                    int64   `json:"id"`
	Key                   string  `json:"-"`
	Service               int     `json:"service"`
	Protocol              string  `json:"protocol"`
	Start                 int64   `json:"startTimestamp"`
	End                   int64   `json:"endTimestamp"`
	Patterns              []int   `json:"foundPatternsIds"`
	Favorite              bool    `json:"favorite"`
	TTL                   int     `json:"ttl"`
	UserAgentHash         *string `json:"userAgentHash"`
	Size                  int64   `json:"sizeBytes"`
	PacketsCount          int     `json:"packetsCount"`
	Incomplete            bool    `json:"incomplete"`
	Client                string  `json:"client"`
	Server                string  `json:"server"`
}

// Chunk is a bounded, immutable piece of a connection. Token makes retrying a
// transaction safe even when the previous commit response was lost.
type Chunk struct {
	Decoded   bool // Protocol decoding enabled; raw transport copies are not behavior messages.
	Sequence  int64
	Closed    bool
	QueueCost int64
	Enqueued  time.Time
	Token     string
	Stream    Stream
	Packets   []Packet
	Bytes     int64
	Ignore    bool
}

type Page struct {
	PatternIDs     []int  `json:"patternIds"`
	Representation string `json:"representation"`
	StartingFrom   *int64 `json:"startingFrom"`
	PageSize       int    `json:"pageSize"`
	Favorites      bool   `json:"favorites"`
	Pattern        *struct {
		ID any `json:"id"`
	} `json:"pattern"`
}

const MaxPatternFilters = 100

// SelectedPatternIDs combines the multi-select API with its legacy single
// pattern form. Every returned ID is required (AND), and duplicates are inert.
func (p Page) SelectedPatternIDs() ([]int, error) {
	if len(p.PatternIDs) > MaxPatternFilters {
		return nil, fmt.Errorf("at most %d patternIds are allowed", MaxPatternFilters)
	}
	ids := make([]int, 0, len(p.PatternIDs)+1)
	seen := make(map[int]bool, len(p.PatternIDs)+1)
	add := func(id int) error {
		if id < 1 || int64(id) > 2147483647 {
			return fmt.Errorf("patternIds must contain positive 32-bit integers")
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
		return nil
	}
	for _, id := range p.PatternIDs {
		if err := add(id); err != nil {
			return nil, err
		}
	}
	if p.Pattern != nil {
		id, err := strconv.ParseInt(fmt.Sprint(p.Pattern.ID), 10, 32)
		if err != nil || id < 0 {
			return nil, fmt.Errorf("invalid pattern id")
		}
		// Legacy id=0 meant "no single-pattern filter".
		if id != 0 {
			if err = add(int(id)); err != nil {
				return nil, err
			}
		}
	}
	if len(ids) > MaxPatternFilters {
		return nil, fmt.Errorf("at most %d distinct patterns are allowed", MaxPatternFilters)
	}
	return ids, nil
}
