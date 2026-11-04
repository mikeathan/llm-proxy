package netaudit

import (
	"strings"
	"testing"

	"llm-proxy/internal/platform/hostwatch"
)

func patchLook(t *testing.T, p hostwatch.Patches) HostLook {
	t.Helper()
	look, err := newWatcher(&fakeHost{patches: p}, &memHostStore{}).Look(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return look
}

func findingFor(look HostLook, check string) (Finding, bool) {
	for _, f := range look.Findings {
		if f.Check == check {
			return f, true
		}
	}
	return Finding{}, false
}

func TestPatchFindings_VertexAsCapturedOnTwoNovember(t *testing.T) {
	// The server's real state: a restart waiting 5 days, 12 updates, no standard security updates, 29 more with Ubuntu Pro.
	look := patchLook(t, hostwatch.Patches{RebootRequired: true, RebootDays: 5, UpdatesRead: true, Updates: 12, ESMUpdates: 29})
	reboot, ok := findingFor(look, CheckHostRebootPending)
	if !ok || reboot.Severity != SeverityLow || reboot.Device != HostDevice || !strings.Contains(reboot.Evidence, "5 days") {
		t.Fatalf("reboot finding = %+v", reboot)
	}
	updates, ok := findingFor(look, CheckHostUpdatesPending)
	if !ok || updates.Severity != SeverityLow || !strings.Contains(updates.Evidence, "12") || !strings.Contains(updates.Evidence, "29") {
		t.Fatalf("updates finding = %+v", updates)
	}
}

func TestPatchFindings_SeverityRisesWithAgeAndSecurityUpdates(t *testing.T) {
	look := patchLook(t, hostwatch.Patches{RebootRequired: true, RebootDays: rebootStaleDays, UpdatesRead: true, Updates: 4, SecurityUpdates: 1})
	if f, _ := findingFor(look, CheckHostRebootPending); f.Severity != SeverityMedium {
		t.Errorf("a restart waiting %d days is %q, want medium", rebootStaleDays, f.Severity)
	}
	if f, _ := findingFor(look, CheckHostUpdatesPending); f.Severity != SeverityMedium {
		t.Errorf("a waiting security update is %q, want medium", f.Severity)
	}
}

func TestPatchFindings_NothingWaitingAndUnknownAreBothSilent(t *testing.T) {
	if look := patchLook(t, hostwatch.Patches{UpdatesRead: true}); len(look.Findings) != 0 {
		t.Errorf("an up to date computer reported %+v", look.Findings)
	}
	if look := patchLook(t, hostwatch.Patches{}); len(look.Findings) != 0 {
		t.Errorf("a computer whose update state is unknown reported %+v", look.Findings)
	}
}

func TestPatchFindings_UnreadableFilesAreCounted(t *testing.T) {
	if look := patchLook(t, hostwatch.Patches{Unreadable: 1}); look.Unreadable != 1 {
		t.Errorf("Unreadable = %d, want 1", look.Unreadable)
	}
}

func TestPatchFindings_EveryCheckIsRegistered(t *testing.T) {
	for _, check := range []string{CheckHostRebootPending, CheckHostUpdatesPending} {
		if RemediationFor(check) == "" || CheckTitle(check) == "" {
			t.Errorf("%q is not registered", check)
		}
		if g, ok := GuideFor(check); !ok || len(g.Steps) == 0 || g.Risk == "" || g.Undo == "" {
			t.Errorf("%q has no complete guide", check)
		}
	}
}

func TestPatchFindings_AutomaticUpdatesThatHaveNotRunForAWeekAreAFinding(t *testing.T) {
	// Vertex's log (2026-11-02) shows a run every day or so; a run 7 or more days ago is the finding, and a log that says
	// nothing is not.
	for days, want := range map[int]bool{0: false, 6: false, 7: true, 30: true} {
		_, got := findingFor(patchLook(t, hostwatch.Patches{AutoRunRead: true, AutoRunDays: days}), CheckHostAutoUpdatesStalled)
		if got != want {
			t.Errorf("%d days: finding = %v, want %v", days, got, want)
		}
	}
	if _, got := findingFor(patchLook(t, hostwatch.Patches{AutoRunDays: 90}), CheckHostAutoUpdatesStalled); got {
		t.Error("an unread log says nothing")
	}
	f, _ := findingFor(patchLook(t, hostwatch.Patches{AutoRunRead: true, AutoRunDays: 12}), CheckHostAutoUpdatesStalled)
	if f.Severity != SeverityLow || f.Device != HostDevice || !strings.Contains(f.Evidence, "12 days") {
		t.Errorf("finding = %+v", f)
	}
	if RemediationFor(CheckHostAutoUpdatesStalled) == "" || CheckTitle(CheckHostAutoUpdatesStalled) == "" {
		t.Error("not registered")
	}
	if g, ok := GuideFor(CheckHostAutoUpdatesStalled); !ok || len(g.Steps) == 0 || g.Risk == "" || g.Undo == "" {
		t.Errorf("guide = %+v", g)
	}
}
