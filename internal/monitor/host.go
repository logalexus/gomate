// Package monitor collects bounded, shared resource history independently of UI clients.
package monitor

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

type hostCounters struct {
	total, idle, rx, tx uint64
	networks            []string
}

type Host struct {
	Point
	CPUs         int      `json:"cpus"`
	Load1        float64  `json:"load1"`
	Uptime       float64  `json:"uptimeSeconds"`
	Interfaces   []string `json:"interfaces"`
	DiskError    string   `json:"diskError,omitempty"`
	NetworkError string   `json:"networkError,omitempty"`
	Error        string   `json:"error,omitempty"`
}

// The explicit mounts avoid accidentally reporting the gateway container's filesystem.
func readHost(proc, root string, now time.Time) (Host, hostCounters, error) {
	h := Host{Point: Point{Time: now.UnixMilli()}}
	var c hostCounters
	b, err := os.ReadFile(filepath.Join(proc, "stat"))
	if err != nil {
		return h, c, fmt.Errorf("Host metrics unavailable: check the /host/proc mount")
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if f[0] == "cpu" {
			// guest/guest_nice are already included in user/nice.
			if len(f) < 9 {
				return h, c, fmt.Errorf("Invalid host CPU counters")
			}
			for i, s := range f[1:9] {
				v, e := strconv.ParseUint(s, 10, 64)
				if e != nil {
					return h, c, e
				}
				c.total += v
				if i == 3 || i == 4 {
					c.idle += v
				}
			}
		} else if strings.HasPrefix(f[0], "cpu") {
			h.CPUs++
		}
	}
	if c.total == 0 || h.CPUs == 0 {
		return h, c, fmt.Errorf("Host CPU counters unavailable")
	}
	b, err = os.ReadFile(filepath.Join(proc, "meminfo"))
	if err != nil {
		return h, c, err
	}
	mem := make(map[string]uint64)
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		v, _ := strconv.ParseUint(f[1], 10, 64)
		mem[strings.TrimSuffix(f[0], ":")] = v * 1024
	}
	available, ok := mem["MemAvailable"]
	if !ok || mem["MemTotal"] == 0 || available > mem["MemTotal"] {
		return h, c, fmt.Errorf("Host memory counters unavailable")
	}
	h.MemoryUsed, h.MemoryLimit = ptr(float64(mem["MemTotal"]-available)), ptr(float64(mem["MemTotal"]))
	if b, err = os.ReadFile(filepath.Join(proc, "loadavg")); err == nil {
		fmt.Sscanf(string(b), "%f", &h.Load1)
	}
	if b, err = os.ReadFile(filepath.Join(proc, "uptime")); err == nil {
		fmt.Sscanf(string(b), "%f", &h.Uptime)
	}
	b, err = os.ReadFile(filepath.Join(proc, "net/dev"))
	if err == nil {
		scan := bufio.NewScanner(strings.NewReader(string(b)))
		for scan.Scan() {
			name, fields, found := strings.Cut(scan.Text(), ":")
			name = strings.TrimSpace(name)
			if !found || excludedInterface(name) {
				continue
			}
			f := strings.Fields(fields)
			if len(f) < 16 {
				continue
			}
			rx, e1 := strconv.ParseUint(f[0], 10, 64)
			tx, e2 := strconv.ParseUint(f[8], 10, 64)
			if e1 != nil || e2 != nil {
				continue
			}
			c.rx += rx
			c.tx += tx
			c.networks = append(c.networks, name)
		}
		h.Interfaces = c.networks
		if len(c.networks) == 0 {
			h.NetworkError = "No network counters for the selected interfaces"
		}
	} else {
		h.NetworkError = "Host network counters unavailable"
	}
	var fs unix.Statfs_t
	if root == "" || unix.Statfs(root, &fs) != nil || fs.Blocks == 0 {
		h.DiskError = "Host disk unavailable: check the /host/root mount"
	} else {
		h.DiskUsed = ptr(float64(fs.Blocks-fs.Bfree) * float64(fs.Bsize))
		h.DiskTotal = ptr(float64(fs.Blocks) * float64(fs.Bsize))
		h.DiskAvailable = ptr(float64(fs.Bavail) * float64(fs.Bsize))
	}
	return h, c, nil
}

func excludedInterface(name string) bool {
	for _, prefix := range []string{"veth", "docker", "br-", "virbr"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return name == "lo"
}
