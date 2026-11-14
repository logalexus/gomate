package model

type RuntimeSettings struct {
	AnomalyTimeoutSeconds   int64    `json:"anomalyTimeoutSeconds"`
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

// Older saved settings omit this field. Keep the original grouping budget.
func (v RuntimeSettings) AnomalyTimeout() int64 {
	if v.AnomalyTimeoutSeconds == 0 {
		return 10
	}
	return v.AnomalyTimeoutSeconds
}
