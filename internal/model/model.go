package model

import "time"

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

type Packet struct {
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
	PayloadPurged bool    `json:"payloadPurged"`
	ID            int64   `json:"id"`
	Key           string  `json:"-"`
	Service       int     `json:"service"`
	Protocol      string  `json:"protocol"`
	Start         int64   `json:"startTimestamp"`
	End           int64   `json:"endTimestamp"`
	Patterns      []int   `json:"foundPatternsIds"`
	Favorite      bool    `json:"favorite"`
	TTL           int     `json:"ttl"`
	UserAgentHash *string `json:"userAgentHash"`
	Size          int64   `json:"sizeBytes"`
	PacketsCount  int     `json:"packetsCount"`
	Incomplete    bool    `json:"incomplete"`
	Client        string  `json:"client"`
	Server        string  `json:"server"`
}

// Chunk is a bounded, immutable piece of a connection. Token makes retrying a
// transaction safe even when the previous commit response was lost.
type Chunk struct {
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
	Representation string `json:"representation"`
	StartingFrom   *int64 `json:"startingFrom"`
	PageSize       int    `json:"pageSize"`
	Favorites      bool   `json:"favorites"`
	Pattern        *struct {
		ID any `json:"id"`
	} `json:"pattern"`
}
