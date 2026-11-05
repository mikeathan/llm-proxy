package netaudit

import (
	"net/netip"
	"testing"
)

func TestOwnModelServer_RelabelsOnlyThisProgramsOwnModelPort(t *testing.T) {
	own := OwnModelServer{
		IsOwnAddr: func(a netip.Addr) bool { return a == netip.MustParseAddr("192.168.50.60") },
		Ports:     func() []int { return []int{8081} },
	}
	findings := []Finding{
		{Device: "192.168.50.60", Port: 8081, Proto: "tcp", Check: CheckAIAPIExposed, Severity: SeverityHigh},  // ours
		{Device: "192.168.50.60", Port: 11434, Proto: "tcp", Check: CheckAIAPIExposed, Severity: SeverityHigh}, // another program here
		{Device: "192.168.50.52", Port: 8081, Proto: "tcp", Check: CheckAIAPIExposed, Severity: SeverityHigh},  // another device
		{Device: "192.168.50.60", Port: 8081, Proto: "tcp", Check: CheckOpenPort, Severity: SeverityInfo},
	}
	want := []string{CheckOwnModelServerOpen, CheckAIAPIExposed, CheckAIAPIExposed, CheckOpenPort}
	got := own.Relabel(findings)
	if got[0].Evidence != ownModelServerWords || got[0].Severity != SeverityHigh {
		t.Errorf("our own model port = %+v; want the own-model-server finding", got[0])
	}
	for i, f := range got {
		if f.Check != want[i] {
			t.Errorf("finding %d check = %q, want %q", i, f.Check, want[i])
		}
	}
	if CheckTitle(CheckOwnModelServerOpen) == "" || RemediationFor(CheckOwnModelServerOpen) == "" {
		t.Error("the new check needs a title and a fix")
	}
	if _, ok := GuideFor(CheckOwnModelServerOpen); !ok {
		t.Error("the new check needs a guide")
	}
}

func TestOwnModelServer_WithoutSourcesChangesNothing(t *testing.T) {
	f := []Finding{{Device: "192.168.50.60", Port: 8081, Check: CheckAIAPIExposed}}
	if got := (OwnModelServer{}).Relabel(f); got[0].Check != CheckAIAPIExposed {
		t.Errorf("got %+v", got[0])
	}
}

// A night the AI-server probe cannot judge this computer's model port keeps the earlier own-model-server finding, as it keeps
// any probe's finding: the stored finding carries the relabelled check, and must not be dropped (resolved one night, new the next).
func TestOwnModelServer_AnUnsureProbeKeepsTheRelabelledFinding(t *testing.T) {
	earlier := []Finding{{Device: "192.168.50.60", Port: 8081, Proto: "tcp", Check: CheckOwnModelServerOpen, Severity: SeverityHigh}}
	unsure := ServiceResult{unknown: map[string]bool{serviceKey("192.168.50.60", 8081, CheckAIAPIExposed): true}}
	if got := unsure.Carry(earlier); len(got) != 1 || got[0].Check != CheckOwnModelServerOpen {
		t.Errorf("carried %+v; want the own-model-server finding kept", got)
	}
}
