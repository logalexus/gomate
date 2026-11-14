package config

import (
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DiscoveryURL, DiscoveryToken                                  string
	Listen, DatabaseURL, Login, Password, Interface, PCAP, WebDir string
	LocalIPs                                                      []netip.Addr
	Shards, QueuePackets, MaxFlows, MaxFlowBytes                  int
	ActiveBytes, QueueBytes, WriteBytes                           int64
	FlushInterval, IdleTimeout, DBTimeout, Retention              time.Duration
}

func Load() (Config, error) {
	c := Config{DiscoveryURL: os.Getenv("DOCKER_DISCOVERY_URL"), DiscoveryToken: os.Getenv("DOCKER_DISCOVERY_TOKEN"), Listen: env("LISTEN", "127.0.0.1:65000"), DatabaseURL: os.Getenv("DATABASE_URL"), Login: env("WEB_LOGIN", "packmate"), Password: os.Getenv("WEB_PASSWORD"), Interface: os.Getenv("INTERFACE"), PCAP: os.Getenv("PCAP_FILE"), WebDir: env("WEB_DIR", "web/dist"), Shards: 2, QueuePackets: 2048, MaxFlows: 4096, MaxFlowBytes: 256 << 10, ActiveBytes: 64 << 20, QueueBytes: 32 << 20, WriteBytes: 64 << 20, FlushInterval: time.Second, IdleTimeout: 40 * time.Second, DBTimeout: 3 * time.Second, Retention: time.Hour}
	for name, dst := range map[string]*int{"SHARDS": &c.Shards, "QUEUE_PACKETS": &c.QueuePackets, "MAX_FLOWS": &c.MaxFlows, "MAX_FLOW_BYTES": &c.MaxFlowBytes} {
		if v := os.Getenv(name); v != "" {
			n, e := strconv.Atoi(v)
			if e != nil || n < 1 {
				return c, fmt.Errorf("invalid %s", name)
			}
			*dst = n
		}
	}
	for name, dst := range map[string]*int64{"ACTIVE_BYTES": &c.ActiveBytes, "QUEUE_BYTES": &c.QueueBytes, "WRITE_BYTES": &c.WriteBytes} {
		if v := os.Getenv(name); v != "" {
			n, e := strconv.ParseInt(v, 10, 64)
			if e != nil || n < 1 {
				return c, fmt.Errorf("invalid %s", name)
			}
			*dst = n
		}
	}
	for name, dst := range map[string]*time.Duration{"FLUSH_INTERVAL": &c.FlushInterval, "IDLE_TIMEOUT": &c.IdleTimeout, "DB_TIMEOUT": &c.DBTimeout, "RETENTION": &c.Retention} {
		if v := os.Getenv(name); v != "" {
			d, e := time.ParseDuration(v)
			if e != nil || d <= 0 {
				return c, fmt.Errorf("invalid %s", name)
			}
			*dst = d
		}
	}
	for _, v := range strings.Split(os.Getenv("LOCAL_IPS"), ",") {
		if v = strings.TrimSpace(v); v != "" {
			a, e := netip.ParseAddr(v)
			if e != nil {
				return c, fmt.Errorf("LOCAL_IPS: %w", e)
			}
			c.LocalIPs = append(c.LocalIPs, a.Unmap())
		}
	}
	if c.DatabaseURL == "" || c.Password == "" {
		return c, fmt.Errorf("DATABASE_URL and WEB_PASSWORD are required")
	}
	if c.Interface != "" && c.PCAP != "" {
		return c, fmt.Errorf("choose INTERFACE or PCAP_FILE, not both")
	}
	if c.PCAP != "" && len(c.LocalIPs) == 0 {
		return c, fmt.Errorf("LOCAL_IPS is required for capture")
	}
	if c.Shards > 32 || c.MaxFlows < c.Shards || c.MaxFlows > 65536 || c.MaxFlowBytes > 4<<20 || c.QueuePackets > 65536 {
		return c, fmt.Errorf("resource limits outside supported range")
	}
	return c, nil
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
