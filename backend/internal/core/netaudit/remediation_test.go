package netaudit

import (
	"strings"
	"testing"
)

// allChecks is every check name checks.go declares; a new check is added here and gets its fix in remediation.go.
var allChecks = []string{
	CheckSMBv1, CheckSMBSigning, CheckFTPAnonymous, CheckVNCNoAuth, CheckRedisNoAuth,
	CheckNFSExports, CheckCertExpired, CheckCertExpiring, CheckSSHWeakAlgorithms, CheckRDPNoNLA,
}

func TestRemediationFor_EveryCheckHasAFix(t *testing.T) {
	for _, check := range allChecks {
		if RemediationFor(check) == "" {
			t.Errorf("check %q has no suggested fix", check)
		}
	}
}

func TestRemediationFor_OpenPortAndUnknownHaveNone(t *testing.T) {
	for _, check := range []string{CheckOpenPort, "no_such_check", ""} {
		if got := RemediationFor(check); got != "" {
			t.Errorf("RemediationFor(%q) = %q, want empty", check, got)
		}
	}
}

func TestRemediationFor_IsOneSafeLine(t *testing.T) {
	for _, check := range allChecks {
		fix := RemediationFor(check)
		if strings.ContainsAny(fix, "\n|`<>") {
			t.Errorf("fix for %q is not plain one-line text: %q", check, fix)
		}
	}
}

func TestRenderObserved_ChecksTableCarriesTheFix(t *testing.T) {
	findings := []Finding{{Device: "10.0.0.5", Port: 445, Proto: "tcp", Check: CheckSMBv1, Severity: SeverityHigh, Evidence: "SMBv1 is enabled"}}
	got := RenderObserved(Observed{Result: Result{Findings: findings}})
	if !strings.Contains(got, "| Suggested fix |") || !strings.Contains(got, RemediationFor(CheckSMBv1)) {
		t.Errorf("checks table lacks the suggested fix:\n%s", got)
	}
}
