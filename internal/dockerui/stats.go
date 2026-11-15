package dockerui

import (
	"context"
	"time"
)

// Stats is deliberately narrower than the Docker response (no process/config data).
type Stats struct {
	Read time.Time `json:"read"`
	CPU  struct {
		Usage struct {
			Total uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		System uint64 `json:"system_cpu_usage"`
		CPUs   uint64 `json:"online_cpus"`
	} `json:"cpu_stats"`
	Memory struct {
		Usage uint64            `json:"usage"`
		Limit uint64            `json:"limit"`
		Stats map[string]uint64 `json:"stats"`
	} `json:"memory_stats"`
	Networks map[string]struct {
		RX uint64 `json:"rx_bytes"`
		TX uint64 `json:"tx_bytes"`
	} `json:"networks"`
	Block struct {
		Entries []struct {
			Op    string `json:"op"`
			Value uint64 `json:"value"`
		} `json:"io_service_bytes_recursive"`
	} `json:"blkio_stats"`
	Pids struct {
		Current uint64 `json:"current"`
	} `json:"pids_stats"`
}

func (e *Engine) Stats(ctx context.Context, id string) (Stats, error) {
	var v Stats
	if !ValidID(id) {
		return v, &Error{400, "Некорректный ID контейнера"}
	}
	resp, err := e.request(ctx, "GET", "/containers/"+id+"/stats?stream=false&one-shot=true")
	if err != nil {
		return v, err
	}
	defer resp.Body.Close()
	err = boundedJSON(resp.Body, &v)
	return v, err
}
