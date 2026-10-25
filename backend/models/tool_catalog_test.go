package models

import (
	"slices"
	"testing"
)

func TestToolCatalog_NamesAndCategoriesAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, spec := range toolCatalog {
		if seen[spec.Name] {
			t.Errorf("tool %q declared twice", spec.Name)
		}
		seen[spec.Name] = true
		if !IsToolCategory(spec.Category) {
			t.Errorf("tool %q has unknown category %q", spec.Name, spec.Category)
		}
	}
}

func TestToolCatalog_ReachImpliesGate(t *testing.T) {
	for _, spec := range toolCatalog {
		if spec.Reach != ReachNone && spec.Gate == GateNone {
			t.Errorf("%s reaches the network but no guardrail gate hides it", spec.Name)
		}
	}
}

func TestEffectOf(t *testing.T) {
	if got := EffectOf("some_mcp_tool"); got != EffectMutating {
		t.Errorf("an unlisted tool must be mutating, got %v", got)
	}
	if got := EffectOf(ToolMemoryUpdate); got != EffectHousekeeping {
		t.Errorf("memory_update = %v, want housekeeping", got)
	}
}

func TestGateOf(t *testing.T) {
	if got := ToolsWithGate(GateNetwork); !slices.Equal(got, []string{ToolNetworkFetch, ToolNetworkInfo, ToolNetworkScan}) {
		t.Errorf("network gate tools = %v", got)
	}
	if GateOf(ToolTerminalExecute) != GateNone {
		t.Error("terminal has no gate")
	}
}

func TestScopeForbiddenTools(t *testing.T) {
	all := []string{ToolNetworkFetch, ToolNetworkScan, ToolNetworkInfo, ToolInternetSearch, ToolNotifyUser}
	cases := []struct {
		scope NetworkScope
		want  []string
	}{
		{NetworkScopeNone, all},
		{NetworkScopeLan, []string{ToolInternetSearch, ToolNotifyUser}},
		{NetworkScopeInternetOnly, []string{ToolNetworkScan, ToolNetworkInfo}},
		{NetworkScopeInternet, nil},
		{NetworkScopeInherit, nil},
	}
	for _, tt := range cases {
		got := ScopeForbiddenTools(tt.scope)
		slices.Sort(got)
		want := slices.Clone(tt.want)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("ScopeForbiddenTools(%q) = %v, want %v", tt.scope, got, want)
		}
	}
	if got := HostNetworkGatedTools(); len(got) != len(all) {
		t.Errorf("host-gated tools = %v", got)
	}
}
