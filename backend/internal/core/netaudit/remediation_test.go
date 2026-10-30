package netaudit

import (
	"strings"
	"testing"
)

// allChecks is every check name checks.go declares; a new check is added here and gets its fix in remediation.go.
var allChecks = []string{
	CheckSMBv1, CheckSMBSigning, CheckFTPAnonymous, CheckVNCNoAuth, CheckRedisNoAuth,
	CheckNFSExports, CheckCertExpired, CheckCertExpiring, CheckSSHWeakAlgorithms, CheckRDPNoNLA, CheckKnownVulns,
	CheckSSHWeakHostKey, CheckTLSWeakKey, CheckTLSWeakSignature, CheckMemcachedOpen, CheckMongoNoAuth, CheckMQTTAnonymous,
	CheckTelnetOpen, CheckDockerAPI, CheckUnexpectedPort, CheckExpectedPortClosed,
	CheckTLSWeakProtocol, CheckTLSWeakCipher, CheckSSHPasswordLogin,
	CheckADBExposed, CheckOpenProxy, CheckElasticsearchExposed, CheckX11Exposed, CheckRsyncExposed, CheckKubeletExposed, CheckEtcdExposed,
	CheckNewDevice, CheckMACChanged, CheckGatewayChanged, CheckDuplicateMAC, CheckDeviceMissing, CheckHiddenDevice,
	CheckHostKeyChanged, CheckCertChanged,
}

func TestRemediationFor_EveryCheckHasAFix(t *testing.T) {
	for _, check := range allChecks {
		if RemediationFor(check) == "" {
			t.Errorf("check %q has no suggested fix", check)
		}
	}
}

func TestCheckTitle_EveryCheckHasAShortName(t *testing.T) {
	for _, check := range allChecks {
		title := CheckTitle(check)
		if title == "" || strings.ContainsAny(title, "\n|`<>") {
			t.Errorf("check %q needs a short plain title, got %q", check, title)
		}
	}
	for _, check := range []string{CheckOpenPort, "no_such_check", ""} {
		if got := CheckTitle(check); got != "" {
			t.Errorf("CheckTitle(%q) = %q, want empty", check, got)
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

func TestRenderObserved_TheFixIsWrittenOncePerCheckNotOncePerPlace(t *testing.T) {
	var findings []Finding
	for _, device := range []string{"10.0.0.5", "10.0.0.6", "10.0.0.7"} {
		findings = append(findings, Finding{Device: device, Port: 445, Proto: "tcp", Check: CheckSMBv1, Severity: SeverityHigh, Evidence: "SMBv1 is enabled"})
	}
	got := RenderObserved(Observed{Result: Result{Findings: findings}})
	if n := strings.Count(got, RemediationFor(CheckSMBv1)); n != 1 {
		t.Errorf("the fix must appear once under the table, not once per device (%d times):\n%s", n, got)
	}
	if strings.Count(got, "SMBv1 is enabled") != 1 || !strings.Contains(got, "10.0.0.5 445/tcp, 10.0.0.6 445/tcp, 10.0.0.7 445/tcp") {
		t.Errorf("the same problem on three devices is one row that names all three:\n%s", got)
	}
	if !strings.Contains(got, "**"+CheckTitle(CheckSMBv1)+"**") {
		t.Errorf("the fix is labelled with the check's title:\n%s", got)
	}
}
