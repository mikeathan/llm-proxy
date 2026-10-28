package netaudit

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/platform/progexec"
	"llm-proxy/internal/platform/sandbox"
	"llm-proxy/models"
)

type fakeNmap struct{ err error }

func (f fakeNmap) Available(context.Context) error { return f.err }

type fakeSB struct{ m sandbox.Enforcement }

func (f fakeSB) Effective() sandbox.EffectiveState {
	return sandbox.EffectiveState{Filesystem: sandbox.SurfaceState{Mechanism: f.m}}
}

func codes(reqs []models.Requirement) []models.RequirementCode {
	var out []models.RequirementCode
	for _, r := range reqs {
		out = append(out, r.Code)
	}
	return out
}

func readyDeps() ReadinessDeps {
	return ReadinessDeps{
		Config:        func() models.NetauditConfig { return models.NetauditConfig{ScanRanges: []string{"192.168.1.0/24"}} },
		Nmap:          fakeNmap{},
		Sandbox:       fakeSB{m: sandbox.EnforcementLandlock},
		HostNetworkOn: func() bool { return true },
	}
}

func TestReadiness_ReadyHostHasNothingMissing(t *testing.T) {
	r := NewReadiness(readyDeps())
	if got := r.Check(context.Background()); len(got) != 0 {
		t.Errorf("requirements = %+v", got)
	}
}

func TestReadiness_BlockingIsSettingsOnly(t *testing.T) {
	deps := readyDeps()
	deps.Config = func() models.NetauditConfig { return models.NetauditConfig{} }
	deps.HostNetworkOn = func() bool { return false }
	deps.Nmap = fakeNmap{err: errors.New("must not be consulted")}
	got := NewReadiness(deps).Blocking()
	if !slices.Equal(codes(got), []models.RequirementCode{models.RequirementHostNetworkOff, models.RequirementScopeEmpty}) {
		t.Fatalf("blocking = %v", codes(got))
	}
	for _, req := range got {
		if !req.Blocking || req.Fix == nil || req.Fix.Kind != models.FixOpenSettings {
			t.Errorf("%s must block and point at a settings section: %+v", req.Code, req)
		}
	}
	if got[1].Fix.Section != "network-scan" {
		t.Errorf("scope fix = %+v", got[1].Fix)
	}
}

func TestReadiness_MissingNmapOffersTheInstallCommandAndDoesNotBlock(t *testing.T) {
	deps := readyDeps()
	deps.Nmap = fakeNmap{err: &progexec.NotInstalledError{Program: "nmap", Hint: "brew install nmap"}}
	got := NewReadiness(deps).Check(context.Background())
	if len(got) != 1 || got[0].Code != models.RequirementNmapMissing || got[0].Blocking {
		t.Fatalf("requirements = %+v", got)
	}
	if got[0].Fix == nil || got[0].Fix.Kind != models.FixCopy || got[0].Fix.Text != "brew install nmap" {
		t.Errorf("fix = %+v", got[0].Fix)
	}
}

func TestReadiness_NoConfinementAsksForAConfirmedOptIn(t *testing.T) {
	deps := readyDeps()
	deps.Sandbox = fakeSB{m: sandbox.EnforcementNone}
	got := NewReadiness(deps).Check(context.Background())
	if len(got) != 1 || got[0].Code != models.RequirementNoSandbox || got[0].Fix.Kind != models.FixConfirm || got[0].Fix.Action != ActionAllowUnconfined {
		t.Fatalf("requirements = %+v", got)
	}

	deps.Config = func() models.NetauditConfig {
		return models.NetauditConfig{ScanRanges: []string{"192.168.1.0/24"}, AllowUnconfined: true}
	}
	if got := NewReadiness(deps).Check(context.Background()); len(got) != 0 {
		t.Errorf("once the operator allows it nothing is missing: %+v", got)
	}
}

func TestReadiness_NoSandboxIsNotReportedWhenNmapIsMissingToo(t *testing.T) {
	deps := readyDeps()
	deps.Sandbox = fakeSB{m: sandbox.EnforcementNone}
	deps.Nmap = fakeNmap{err: errors.New("gone")}
	got := NewReadiness(deps).Check(context.Background())
	if !slices.Equal(codes(got), []models.RequirementCode{models.RequirementNmapMissing}) {
		t.Errorf("requirements = %v (confinement only matters once nmap is there)", codes(got))
	}
}

func TestReadiness_OldVulnerabilityDataIsANoticeThatOffersTheUpdate(t *testing.T) {
	deps := readyDeps()
	deps.VulnData = func() VulnDataView { return VulnDataView{FilesDone: 6, FilesTotal: 6, Age: 61 * 24 * time.Hour} }
	r := NewReadiness(deps)
	got := r.Notices()
	if len(got) != 1 || got[0].Code != models.RequirementVulnDataOld || got[0].Blocking || !strings.Contains(got[0].Message, "61 days") ||
		got[0].Fix == nil || got[0].Fix.Kind != models.FixOpenSettings || got[0].Fix.Section != "network-scan" {
		t.Errorf("notices = %+v", got)
	}
	if len(r.Check(context.Background())) != 0 {
		t.Error("old data must not count as an unmet requirement")
	}
	if _, _, refused := r.Preflight(context.Background(), AuditorProfileID); refused {
		t.Error("a scheduled audit must still run with old data")
	}
}

func TestReadiness_NoticesSayWhatTheVulnerabilityDataLacksButNeverStopAnAudit(t *testing.T) {
	cases := []struct {
		name string
		view VulnDataView
		want string // "" = no notice
	}{
		{"nothing downloaded", VulnDataView{FilesTotal: 6}, "not downloaded"},
		{"running", VulnDataView{Running: true, FilesDone: 2, FilesTotal: 6}, "downloading (2 of 6 files)"},
		{"stopped part-way", VulnDataView{FilesDone: 3, FilesTotal: 6}, "did not finish (3 of 6 files)"},
		{"complete", VulnDataView{FilesDone: 6, FilesTotal: 6}, ""},
		{"complete and recent", VulnDataView{FilesDone: 6, FilesTotal: 6, Age: 59 * 24 * time.Hour}, ""},
	}
	for _, c := range cases {
		deps := readyDeps()
		view := c.view
		deps.VulnData = func() VulnDataView { return view }
		r := NewReadiness(deps)
		got := r.Notices()
		if c.want == "" {
			if len(got) != 0 {
				t.Errorf("%s: %+v", c.name, got)
			}
		} else if len(got) != 1 || !strings.Contains(got[0].Message, c.want) || got[0].Blocking ||
			got[0].Code != models.RequirementVulnDataMissing || got[0].Fix == nil || got[0].Fix.Kind != models.FixOpenSettings || got[0].Fix.Section != "network-scan" {
			t.Errorf("%s: %+v", c.name, got)
		}
		if len(r.Check(context.Background())) != 0 {
			t.Errorf("%s: a notice must not count as an unmet requirement", c.name)
		}
		if _, _, refused := r.Preflight(context.Background(), AuditorProfileID); refused {
			t.Errorf("%s: a scheduled audit must still run without vulnerability data", c.name)
		}
	}
	if got := NewReadiness(readyDeps()).Notices(); got != nil {
		t.Errorf("no data source wired: %+v", got)
	}
}
