package findingsview

import (
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/models"
)

func at(h time.Duration) time.Time {
	return time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC).Add(h)
}

// A deep finding is carried into every audit without being checked again, so "you marked it fixed and a scan still shows
// it" is judged against the deep check's own time: an audit that merely carried it does not count as a re-check.
func TestOf_FixStateOfADeepFindingFollowsTheDeepCheckNotTheNightlyScan(t *testing.T) {
	marked := at(0)
	deep := models.ScanFinding{Device: "10.0.0.5", Port: 22, Proto: "tcp", Check: netaudit.CheckSSHPasswordLogin, Severity: "medium", Evidence: "e"}
	ordinary := models.ScanFinding{Device: "10.0.0.5", Port: 445, Proto: "tcp", Check: netaudit.CheckSMBv1, Severity: "high", Evidence: "e"}
	fixes := map[string]models.FixRecord{deep.Key(): {Key: deep.Key(), AppliedAt: marked}, ordinary.Key(): {Key: ordinary.Key(), AppliedAt: marked}}
	base := models.ScanBaseline{ScannedAt: at(10 * time.Hour), Findings: []models.ScanFinding{deep, ordinary}}

	state := func(deepAt time.Time) map[string]string {
		rep := Of(Input{Baseline: base, Fixes: fixes, DeepAt: map[string]time.Time{"10.0.0.5": deepAt}})
		out := map[string]string{}
		for _, f := range rep.Findings {
			out[f.Key] = f.FixState
		}
		return out
	}
	before := state(marked.Add(-time.Hour))
	if before[deep.Key()] != string(netaudit.FixMarked) {
		t.Errorf("the deep check is older than the mark, so nothing has re-checked it: %q", before[deep.Key()])
	}
	if before[ordinary.Key()] != string(netaudit.FixStillPresent) {
		t.Errorf("an ordinary finding is judged by the nightly scan: %q", before[ordinary.Key()])
	}
	if after := state(marked.Add(time.Hour)); after[deep.Key()] != string(netaudit.FixStillPresent) {
		t.Errorf("a deep check after the mark that still shows it: %q", after[deep.Key()])
	}
}

func TestOf_AnAuditNeverScannedIsEmptyNotNil(t *testing.T) {
	rep := Of(Input{})
	if rep.Scanned || rep.Findings == nil || rep.Resolved == nil || rep.Unscanned == nil {
		t.Errorf("unscanned report must have empty lists, got %+v", rep)
	}
}

func TestOf_MarksNewAcceptedAndChangedFindings(t *testing.T) {
	f := models.ScanFinding{Device: "10.0.0.5", Port: 22, Proto: "tcp", Check: netaudit.CheckOpenPort, Severity: "info", Version: "9.7"}
	base := models.ScanBaseline{ScannedAt: at(0), Findings: []models.ScanFinding{f}, NewKeys: []string{f.Key()}, ChangedFrom: map[string]string{f.Key(): "9.6"}}
	rep := Of(Input{Baseline: base, Now: at(time.Hour)})
	got := rep.Findings[0]
	if !got.New || !got.Changed || got.PreviousVersion != "9.6" || got.Accepted {
		t.Errorf("new/changed flags wrong: %+v", got)
	}
}

func TestOf_ShowsHowLongAFindingHasBeenThereAndThatItCameBack(t *testing.T) {
	f := models.ScanFinding{Device: "10.0.0.5", Port: 445, Proto: "tcp", Check: netaudit.CheckSMBv1, Severity: "high"}
	gone := models.ScanFinding{Device: "10.0.0.6", Port: 445, Proto: "tcp", Check: netaudit.CheckSMBv1, Severity: "high"}
	base := models.ScanBaseline{ScannedAt: at(0), Findings: []models.ScanFinding{f}, Resolved: []models.ScanFinding{gone}}
	life := map[string]models.FindingLife{
		f.Key():    {Key: f.Key(), FirstSeen: at(-72 * time.Hour), OpenSince: at(-24 * time.Hour), Returned: 2},
		gone.Key(): {Key: gone.Key(), FirstSeen: at(-72 * time.Hour), ResolvedAt: at(-time.Hour), ResolvedBy: models.LifeByRecheck},
	}
	rep := Of(Input{Baseline: base, Life: life})
	got := rep.Findings[0]
	if got.CameBack != 2 || got.FirstSeen != at(-72*time.Hour).UTC().Format(time.RFC3339) || got.OpenSince != at(-24*time.Hour).UTC().Format(time.RFC3339) {
		t.Errorf("history on the finding: %+v", got)
	}
	if r := rep.Resolved[0]; r.ResolvedBy != models.LifeByRecheck || r.ResolvedAt == "" {
		t.Errorf("history on the resolved finding: %+v", r)
	}
}

// A recheck that found the problem gone after the last full scan is shown as verified fixed; one before the scan is not, since
// the scan that followed it still shows the finding.
func TestOf_AFindingARecheckFoundGoneAfterTheScanIsVerifiedFixed(t *testing.T) {
	f := models.ScanFinding{Device: "10.0.0.5", Port: 445, Proto: "tcp", Check: netaudit.CheckSMBv1, Severity: "high"}
	base := models.ScanBaseline{ScannedAt: at(0), Findings: []models.ScanFinding{f}}
	closed := func(when time.Time) map[string]models.FindingLife {
		return map[string]models.FindingLife{f.Key(): {Key: f.Key(), FirstSeen: at(-time.Hour), ResolvedAt: when, ResolvedBy: models.LifeByRecheck}}
	}
	if got := Of(Input{Baseline: base, Life: closed(at(time.Hour))}).Findings[0].VerifiedFixedAt; got == "" {
		t.Error("a recheck after the scan must show as verified fixed")
	}
	if got := Of(Input{Baseline: base, Life: closed(at(-time.Minute))}).Findings[0].VerifiedFixedAt; got != "" {
		t.Errorf("a recheck before the scan is overtaken by it: %q", got)
	}
}

func TestOf_OnlyWhatASingleDeviceScanCanJudgeIsRecheckable(t *testing.T) {
	smb := models.ScanFinding{Device: "10.0.0.5", Port: 445, Proto: "tcp", Check: netaudit.CheckSMBv1, Severity: "high"}
	deep := models.ScanFinding{Device: "10.0.0.5", Port: 22, Proto: "tcp", Check: netaudit.CheckSSHPasswordLogin, Severity: "medium"}
	rep := Of(Input{Baseline: models.ScanBaseline{ScannedAt: at(0), Findings: []models.ScanFinding{smb, deep}}})
	if !rep.Findings[0].Recheckable || rep.Findings[1].Recheckable {
		t.Errorf("recheckable flags: %v %v", rep.Findings[0].Recheckable, rep.Findings[1].Recheckable)
	}
}
