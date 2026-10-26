package netaudit

import (
	"strings"
	"testing"

	"llm-proxy/models"
)

func off() *bool { b := false; return &b }

func TestValidateScanSettings(t *testing.T) {
	tests := []struct {
		name string
		in   models.ScanSettings
		bad  string // part of the refusal; empty means accepted
	}{
		{"the defaults", models.ScanSettings{}, ""},
		{"everything set", models.ScanSettings{Targets: []string{"192.168.1.0/24", "192.168.1.5", "10.0.0.1-10.0.0.9", "nas.local"}, Depth: "quick", Ports: "22,80,8000-8100", Checks: off()}, ""},
		{"an unknown depth", models.ScanSettings{Depth: "deep"}, "depth"},
		{"a port list that could become a flag", models.ScanSettings{Ports: "22;--script=x"}, "ports"},
		{"a public address", models.ScanSettings{Targets: []string{"8.8.8.8"}}, "8.8.8.8"},
		{"an entry that is not an address or a name", models.ScanSettings{Targets: []string{"not an address!"}}, "not an address!"},
		{"an empty entry", models.ScanSettings{Targets: []string{" "}}, "empty"},
		{"too many entries", models.ScanSettings{Targets: make([]string, MaxScanTargets+1)}, "at most"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateScanSettings(tt.in)
			switch {
			case tt.bad == "" && err != nil:
				t.Errorf("unexpected refusal: %v", err)
			case tt.bad != "" && (err == nil || !strings.Contains(err.Error(), tt.bad)):
				t.Errorf("refusal = %v, want it to mention %q", err, tt.bad)
			}
		})
	}
}

func TestRequestFromSettings_TheSettingsDecideEverythingTheModelCouldHave(t *testing.T) {
	req := RequestFromSettings(models.ScanSettings{Targets: []string{"192.168.1.0/24"}, Depth: "quick", Ports: "22,80", Checks: off()})
	if req.Mode != ModeServices || req.Thorough || req.Ports != "22,80" || len(req.Targets) != 1 || req.Targets[0] != "192.168.1.0/24" {
		t.Errorf("quick, checks off: %+v", req)
	}
	req = RequestFromSettings(models.ScanSettings{})
	if req.Mode != ModeChecks || !req.Thorough || req.Ports != "" || len(req.Targets) != 0 {
		t.Errorf("the defaults are a thorough checks scan of every allowed network: %+v", req)
	}
}
