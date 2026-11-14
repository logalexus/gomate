package model

type RuntimeSettings struct {
	Revision                int64    `json:"revision"`
	CaptureEnabled          bool     `json:"captureEnabled"`
	Interface               string   `json:"interface"`
	AutoIPs                 bool     `json:"autoIPs"`
	LocalIPs                []string `json:"localIPs"`
	PacketRetentionSeconds  int64    `json:"packetRetentionSeconds"`
	SessionRetentionSeconds int64    `json:"sessionRetentionSeconds"`
	CleanupIntervalSeconds  int64    `json:"cleanupIntervalSeconds"`
	PreserveFavorites       bool     `json:"preserveFavorites"`
}
