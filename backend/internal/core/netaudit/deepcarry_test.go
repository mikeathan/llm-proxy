package netaudit

import (
	"errors"
	"strings"
	"testing"
	"time"

	"llm-proxy/models"
)

type fakeDeepReader struct {
	items []models.DeepCheck
	err   error
}

func (f *fakeDeepReader) ReadDeepChecks(string) ([]models.DeepCheck, error) { return f.items, f.err }

var deepAt = reportTime.Add(-48 * time.Hour)

func deepFinding(device string, port int, check string) models.ScanFinding {
	return models.ScanFinding{Device: device, Port: port, Proto: "tcp", Service: "ssh", Check: check, Severity: SeverityMedium, Evidence: "The SSH server accepts password login"}
}

func storedDeep(device string, at time.Time, findings ...models.ScanFinding) models.DeepCheck {
	return models.DeepCheck{Device: device, At: at, Ports: []models.DeepPortResult{{Port: 22, SSH: true}}, Findings: findings}
}

// A deep check is not repeated by the nightly scan, so the audit carries its findings: dated, and only while the device is
// up and the port it was found on still answers.
func TestDeepCarry_FindingsJoinTheReportDatedAndStayInTheBaseline(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	deep := &fakeDeepReader{items: []models.DeepCheck{storedDeep("10.0.0.5", deepAt, deepFinding("10.0.0.5", 22, CheckSSHPasswordLogin))}}
	r := reporter(store, reportTime).WithDeepChecks(deep)
	got := r.Augment(scannedHosts(hostOf("10.0.0.5", true, false, 22)), "ws", "nightly", "ok")
	if !strings.Contains(got, "SSH accepts password login") || !strings.Contains(got, "from the deep check of 2026-10-23") {
		t.Errorf("the carried finding is in the report, dated:\n%s", got)
	}
	base := store.data["ws/nightly"].Findings
	var carried *Finding
	for i := range base {
		if base[i].Check == CheckSSHPasswordLogin {
			carried = &base[i]
		}
	}
	if carried == nil || !strings.Contains(carried.Evidence, "from the deep check of 2026-10-23") {
		t.Fatalf("the finding is kept with the scan: %+v", base)
	}
	// The next nightly audit carries it again as the same finding: it is not new twice, and not resolved by not being re-run.
	again := reporter(store, reportTime.Add(24*time.Hour)).WithDeepChecks(deep)
	again.Augment(scannedHosts(hostOf("10.0.0.5", true, false, 22)), "ws", "nightly", "ok")
	next := store.data["ws/nightly"]
	if len(next.NewKeys) != 0 || len(next.Resolved) != 0 {
		t.Errorf("a carried finding is stable between audits: new %v resolved %v", next.NewKeys, next.Resolved)
	}
}

func TestDeepCarry_OnlyWhileTheDeviceIsUpAndThePortStillAnswers(t *testing.T) {
	deep := &fakeDeepReader{items: []models.DeepCheck{
		storedDeep("10.0.0.5", deepAt, deepFinding("10.0.0.5", 22, CheckSSHPasswordLogin)),
		storedDeep("10.0.0.6", deepAt, deepFinding("10.0.0.6", 22, CheckSSHPasswordLogin)),
		storedDeep("10.0.0.7", deepAt, deepFinding("10.0.0.7", 22, CheckSSHPasswordLogin)),
		storedDeep("10.0.0.8", deepAt, deepFinding("10.0.0.8", 22, CheckSSHPasswordLogin)),
	}}
	hosts := scannedHosts(
		hostOf("10.0.0.5", true, false, 22), // up, port open: carried
		hostOf("10.0.0.6", true, false, 80), // the port closed since: the finding is no longer true
		hostOf("10.0.0.7", false, false),    // down tonight: unknown, not carried
		hostOf("10.0.0.8", true, true, 22),  // the scan ran out of time on it: unknown, not carried
	)
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	reporter(store, reportTime).WithDeepChecks(deep).Augment(hosts, "ws", "nightly", "ok")
	var devices []string
	for _, f := range store.data["ws/nightly"].Findings {
		if f.Check == CheckSSHPasswordLogin {
			devices = append(devices, f.Device)
		}
	}
	if len(devices) != 1 || devices[0] != "10.0.0.5" {
		t.Errorf("carried for %v, want only 10.0.0.5", devices)
	}
}

func TestDeepCarry_AnOldResultIsNotCarriedForEver(t *testing.T) {
	old := reportTime.Add(-DeepCarryMaxAge - time.Hour)
	deep := &fakeDeepReader{items: []models.DeepCheck{storedDeep("10.0.0.5", old, deepFinding("10.0.0.5", 22, CheckSSHPasswordLogin))}}
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	got := reporter(store, reportTime).WithDeepChecks(deep).Augment(scannedHosts(hostOf("10.0.0.5", true, false, 22)), "ws", "nightly", "ok")
	if strings.Contains(got, "password login") {
		t.Errorf("a result older than %v is left out:\n%s", DeepCarryMaxAge, got)
	}
}

func TestDeepCarry_OnlyDeepChecksAreCarriedAndTroubleIsStated(t *testing.T) {
	stray := models.ScanFinding{Device: "10.0.0.5", Port: 22, Proto: "tcp", Check: CheckSMBv1, Severity: SeverityHigh, Evidence: "x"}
	deep := &fakeDeepReader{items: []models.DeepCheck{storedDeep("10.0.0.5", deepAt, stray)}}
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	reporter(store, reportTime).WithDeepChecks(deep).Augment(scannedHosts(hostOf("10.0.0.5", true, false, 22)), "ws", "nightly", "ok")
	for _, f := range store.data["ws/nightly"].Findings {
		if f.Check == CheckSMBv1 {
			t.Errorf("a stored file must not smuggle in a finding that is not a deep check: %+v", f)
		}
	}
	bad := &fakeDeepReader{err: errors.New("disk gone")}
	got := reporter(&memBaselines{data: map[string]models.ScanBaseline{}}, reportTime).WithDeepChecks(bad).Augment(scannedHosts(hostOf("10.0.0.5", true, false, 22)), "ws", "nightly", "ok")
	if !strings.Contains(got, "deep check") || !strings.Contains(got, "could not be read") || !strings.Contains(got, "10.0.0.5") {
		t.Errorf("trouble is stated and the scan is still reported:\n%s", got)
	}
}

func TestIsDeepCheck(t *testing.T) {
	for _, c := range []string{CheckTLSWeakProtocol, CheckTLSWeakCipher, CheckSSHPasswordLogin} {
		if !IsDeepCheck(c) {
			t.Errorf("%s is a deep check", c)
		}
	}
	for _, c := range []string{CheckSMBv1, CheckOpenPort, CheckTLSWeakKey, "", "nope"} {
		if IsDeepCheck(c) {
			t.Errorf("%s is not", c)
		}
	}
}
