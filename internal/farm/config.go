package farm

import (
	"fmt"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
)

type Target struct {
	Name string `json:"name"`
	Host string `json:"host"`
}
type Config struct {
	Revision       int64    `json:"revision"`
	Enabled        bool     `json:"enabled"`
	Protocol       string   `json:"protocol"`
	URL            string   `json:"url"`
	Token          string   `json:"token,omitempty"`
	TokenSet       bool     `json:"tokenSet"`
	ClearToken     bool     `json:"clearToken,omitempty"`
	FlagFormat     string   `json:"flagFormat"`
	Targets        []Target `json:"targets"`
	Exclude        []string `json:"exclude"`
	Pattern        string   `json:"pattern"`
	First          int      `json:"first"`
	Last           int      `json:"last"`
	Period         int      `json:"period"`
	Batch          int      `json:"batch"`
	Lifetime       int      `json:"lifetime"`
	Retention      int      `json:"retention"`
	Timeout        int      `json:"timeout"`
	ClientInterval int      `json:"clientInterval"`
	ClientWorkers  int      `json:"clientWorkers"`
}

func Default() Config {
	return Config{Protocol: "ructf_http", FlagFormat: `[A-Z0-9]{31}=`, Targets: []Target{}, Exclude: []string{}, Period: 2, Batch: 100, Lifetime: 300, Retention: 86400, Timeout: 5, ClientInterval: 30, ClientWorkers: 8, First: 1, Last: 50}
}
func (c Config) Public() Config { c.TokenSet = c.Token != ""; c.Token = ""; return c }
func (c Config) Validate() error {
	if c.Protocol != "ructf_http" && c.Protocol != "forcad" && c.Protocol != "volga" {
		return fmt.Errorf("protocol must be ructf_http, forcad or volga")
	}
	if len(c.FlagFormat) > 1024 {
		return fmt.Errorf("flag regex too long")
	}
	re, e := regexp.Compile(c.FlagFormat)
	if e != nil || re.MatchString("") {
		return fmt.Errorf("invalid or empty-matching flag regex")
	}
	if c.URL != "" {
		u, e := url.Parse(c.URL)
		if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("submission URL must be an HTTP(S) URL without credentials or query")
		}
	}
	if c.Enabled && c.URL == "" {
		return fmt.Errorf("submission URL is required")
	}
	if len(c.Token) > 512 || strings.ContainsAny(c.Token, "\r\n") {
		return fmt.Errorf("invalid token")
	}
	if c.Period < 1 || c.Period > 300 || c.Batch < 1 || c.Batch > 100 || c.Timeout < 1 || c.Timeout > 30 || c.Lifetime < 10 || c.Lifetime > 86400 || c.Retention < c.Lifetime || c.Retention > 2592000 || c.ClientWorkers < 1 || c.ClientWorkers > 64 || c.ClientInterval < 1 || c.ClientInterval > 3600 {
		return fmt.Errorf("invalid farm limits")
	}
	_, e = c.Expanded()
	return e
}
func (c Config) Expanded() ([]Target, error) {
	if len(c.Targets) > 2048 || len(c.Exclude) > 2048 {
		return nil, fmt.Errorf("too many targets or exclusions")
	}
	exclude := map[string]bool{}
	for _, s := range c.Exclude {
		a, e := netip.ParseAddr(strings.TrimSpace(s))
		if e != nil {
			return nil, fmt.Errorf("invalid excluded IP: %s", s)
		}
		exclude[a.String()] = true
	}
	all := append([]Target{}, c.Targets...)
	if c.Pattern != "" {
		if strings.Count(c.Pattern, "{n}") != 1 || c.First < 0 || c.Last < c.First || c.Last-c.First > 1023 {
			return nil, fmt.Errorf("use one {n} placeholder and at most 1024 teams")
		}
		for n := c.First; n <= c.Last; n++ {
			all = append(all, Target{fmt.Sprintf("Team #%d", n), strings.Replace(c.Pattern, "{n}", fmt.Sprint(n), 1)})
		}
	}
	out := []Target{}
	seen := map[string]bool{}
	names := map[string]bool{}
	for _, t := range all {
		t.Name = strings.TrimSpace(t.Name)
		a, e := netip.ParseAddr(strings.TrimSpace(t.Host))
		if e != nil || t.Name == "" || len(t.Name) > 128 {
			return nil, fmt.Errorf("each target needs a name and valid IP")
		}
		t.Host = a.String()
		if exclude[t.Host] || seen[t.Host] {
			continue
		}
		if names[t.Name] {
			return nil, fmt.Errorf("duplicate team name: %s", t.Name)
		}
		seen[t.Host] = true
		names[t.Name] = true
		out = append(out, t)
	}
	return out, nil
}
