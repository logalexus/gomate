package discovery

import (
	"encoding/json"
	"testing"
)

func TestPublishedPortsAndInternalContainers(t *testing.T) {
	var cs []Container
	json.Unmarshal([]byte(`[{"Id":"abc","Names":["/api"],"Image":"fixture","State":"running","Ports":[{"IP":"0.0.0.0","PrivatePort":80,"PublicPort":18080,"Type":"tcp"},{"PrivatePort":9090,"Type":"tcp"}]},{"Id":"db","State":"running","Labels":{"gomate.internal":"true"},"Ports":[{"PublicPort":65432,"Type":"tcp"}]},{"Id":"stopped","State":"exited"},{"Id":"host","State":"running","Names":["/host-app"],"Ports":[]}]`), &cs)
	out := Candidates(cs)
	if len(out) != 3 {
		t.Fatal(out)
	}
	available := 0
	for _, c := range out {
		if c.Available {
			available++
			if c.Port != 18080 || c.PrivatePort != 80 || c.Name != "api" {
				t.Fatal(c)
			}
		} else if c.Reason == "" {
			t.Fatal(c)
		}
	}
	if available != 1 {
		t.Fatal(out)
	}
}
