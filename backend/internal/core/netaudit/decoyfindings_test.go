package netaudit

import (
	"slices"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit/decoy"
	"llm-proxy/models"
)

func TestDecoyFindings_AConnectionToADecoyIsHigh(t *testing.T) {
	touches := []models.DecoyTouch{{Source: "192.168.1.77", Port: 2222, First: t0, Last: t0.Add(time.Hour), Count: 3}}
	got := DecoyFindings(touches, t0.Add(2*time.Hour))
	if len(got) != 1 || got[0].Check != CheckDecoyTouched || got[0].Severity != SeverityHigh || got[0].Device != "192.168.1.77" || got[0].Port != 2222 || got[0].Proto != "tcp" {
		t.Fatalf("findings = %+v", got)
	}
	if got[0].Key() != "192.168.1.77|tcp|2222|decoy_touched" {
		t.Errorf("key %q", got[0].Key())
	}
	if !strings.Contains(got[0].Evidence, "3 times") {
		t.Errorf("evidence = %q", got[0].Evidence)
	}
}

func TestDecoyFindings_AnOldTouchIsNotReportedForEver(t *testing.T) {
	touches := []models.DecoyTouch{{Source: "192.168.1.77", Port: 2222, First: t0, Last: t0, Count: 1}}
	if got := DecoyFindings(touches, t0.Add(DecoyReportFor+time.Hour)); len(got) != 0 {
		t.Errorf("got %+v", got)
	}
}

// The decoys must never be reachable by the audit's own scans of the host that runs them, or the audit would raise its own
// alarm and put a fake SSH banner in the identity tracking.
func TestDecoyPortsAreInNoScanList(t *testing.T) {
	for _, s := range decoy.Specs {
		for name, list := range map[string][]int{"DefaultPorts": DefaultPorts, "ThoroughPorts": ThoroughPorts} {
			if slices.Contains(list, s.Number) {
				t.Errorf("decoy port %d is in %s, so the audit would scan it", s.Number, name)
			}
		}
	}
}

func TestDecoyConfig_OffByDefaultAndSaves(t *testing.T) {
	store := &memConfig{}
	c := NewDecoyConfig(store)
	if c.Enabled() {
		t.Fatal("off by default")
	}
	if err := c.Set(true); err != nil || !c.Enabled() || !store.cfg.DecoysEnabled {
		t.Errorf("set: %v %+v", err, store.cfg)
	}
	if err := c.Set(false); err != nil || c.Enabled() {
		t.Errorf("off: %v", err)
	}
}
