package models

import (
	"context"
	"testing"

	"gopkg.in/yaml.v3"
)

func boolPtr(b bool) *bool { return &b }

func TestScanSettings_DefaultsAreThoroughWithChecksOnAcrossEveryAllowedNetwork(t *testing.T) {
	var s ScanSettings
	if !s.Thorough() || !s.ChecksOn() || len(s.Targets) != 0 || s.Ports != "" {
		t.Errorf("the zero value is the usual audit: %+v", s)
	}
	n := s.Normalized()
	if n.Depth != ScanDepthThorough || n.Checks == nil || !*n.Checks {
		t.Errorf("normalised settings say what they are, so a screen needs no defaults of its own: %+v", n)
	}
	if (ScanSettings{Depth: ScanDepthQuick, Checks: boolPtr(false)}).Thorough() || (ScanSettings{Checks: boolPtr(false)}).ChecksOn() {
		t.Error("quick and checks-off must be honoured")
	}
}

func TestAutomation_EffectiveScan(t *testing.T) {
	custom := &ScanSettings{Depth: ScanDepthQuick, Ports: "22,80"}
	if got := (&Automation{Profile: AuditorProfileID, Scan: custom}).EffectiveScan(); got == nil || got.Depth != ScanDepthQuick || got.Ports != "22,80" {
		t.Errorf("an audit's own settings win: %+v", got)
	}
	if got := (&Automation{Profile: AuditorProfileID}).EffectiveScan(); got == nil || got.Depth != ScanDepthThorough || !got.ChecksOn() {
		t.Errorf("an audit with no settings of its own gets the defaults, so the model never decides: %+v", got)
	}
	if got := (&Automation{Profile: "research"}).EffectiveScan(); got != nil {
		t.Errorf("another agent's runs are not constrained: %+v", got)
	}
	if got := (&Automation{}).EffectiveScan(); got != nil {
		t.Errorf("no agent, no settings: %+v", got)
	}
	if got := (&Automation{Profile: "research", Scan: custom}).EffectiveScan(); got == nil {
		t.Error("settings someone chose are kept whatever the agent")
	}
}

func TestScanSettings_ReachTheScanToolThroughTheContext(t *testing.T) {
	if _, ok := ScanSettingsFrom(context.Background()); ok {
		t.Error("a run with no settings has none in its context")
	}
	ctx := WithScanSettings(context.Background(), ScanSettings{Depth: ScanDepthQuick, Targets: []string{"192.168.1.0/24"}})
	got, ok := ScanSettingsFrom(ctx)
	if !ok || got.Depth != ScanDepthQuick || len(got.Targets) != 1 {
		t.Errorf("got %+v, %v", got, ok)
	}
}

func TestAutomation_ScanSurvivesTheConfigFile(t *testing.T) {
	in := Automation{Name: "audit", Scan: &ScanSettings{Targets: []string{"192.168.1.0/24"}, Depth: ScanDepthQuick, Ports: "22,80", Checks: boolPtr(false)}}
	raw, err := yaml.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out Automation
	if err := yaml.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Scan == nil || out.Scan.Depth != ScanDepthQuick || out.Scan.Ports != "22,80" || out.Scan.Checks == nil || *out.Scan.Checks || len(out.Scan.Targets) != 1 {
		t.Errorf("an explicit checks: false must survive a reload, got %+v from %s", out.Scan, raw)
	}
	raw, _ = yaml.Marshal(Automation{Name: "plain"})
	var plain Automation
	_ = yaml.Unmarshal(raw, &plain)
	if plain.Scan != nil {
		t.Errorf("an automation that has none keeps none: %s", raw)
	}
}

func TestScanSettings_KnownVulnsDefaultsOnAndAnExplicitOffSurvivesNormalizing(t *testing.T) {
	if !(ScanSettings{}).KnownVulnsOn() {
		t.Error("no setting must mean on")
	}
	off := false
	s := ScanSettings{KnownVulns: &off}
	if s.KnownVulnsOn() || s.Normalized().KnownVulnsOn() {
		t.Error("an explicit off must stay off")
	}
	if n := (ScanSettings{}).Normalized(); n.KnownVulns == nil || !*n.KnownVulns {
		t.Errorf("Normalized must fill the default in: %+v", n.KnownVulns)
	}
}
