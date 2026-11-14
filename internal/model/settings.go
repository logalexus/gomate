package model

type RuntimeSettings struct {
	RequestTimeoutSeconds   int64    `json:"requestTimeoutSeconds"`
	AnomalyTimeoutSeconds   int64    `json:"anomalyTimeoutSeconds,omitempty"`
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

// Accept the previous anomaly-only setting when upgrading existing installations.
func (v RuntimeSettings) RequestTimeout() int64 {
	if v.RequestTimeoutSeconds != 0 {
		return v.RequestTimeoutSeconds
	}
	if v.AnomalyTimeoutSeconds != 0 {
		return v.AnomalyTimeoutSeconds
	}
	return 10
}
