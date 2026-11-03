package netaudit

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"llm-proxy/models"
)

type fakeNearby struct {
	findings []Finding
	err      error
}

func (f *fakeNearby) Findings() ([]Finding, error) { return f.findings, f.err }

type fakeServices struct {
	found, kept []Finding
}

func (f *fakeServices) Findings(context.Context, []Host) []Finding { return f.found }
func (f *fakeServices) Carry([]Finding) []Finding                  { return f.kept }

func rogue(device string) Finding {
	return Finding{Device: device, Check: CheckRogueGateway, Severity: SeverityMedium, Evidence: rogueGatewayWords}
}

func TestRunReporter_NearbyFindingsJoinTheReportOnlyForScannedDevices(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	n := &fakeNearby{findings: []Finding{rogue("10.0.0.7"), rogue("10.0.0.99")}}
	reporter(store, reportTime).WithNearby(n).Augment(scannedHosts(hostOf("10.0.0.7", true, false, 80)), "ws", "nightly", "")
	seen := false
	for _, f := range store.data["ws/nightly"].Findings {
		seen = seen || (f.Device == "10.0.0.7" && f.Check == CheckRogueGateway)
	}
	if !seen {
		t.Fatal("the finding of a scanned device is missing")
	}
	for _, f := range store.data["ws/nightly"].Findings {
		if f.Device == "10.0.0.99" {
			t.Fatal("a device this scan did not look at got a finding")
		}
	}
}

func TestRunReporter_AFindingFromNearbyStaysThroughALaterRunThatHeardNothing(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	n := &fakeNearby{findings: []Finding{rogue("10.0.0.7")}}
	reporter(store, reportTime.Add(-24*time.Hour)).WithNearby(n).Augment(scannedHosts(hostOf("10.0.0.7", true, false, 80)), "ws", "nightly", "")
	// The kept state still says it (derived from state, not from a look), so the next run reports it unchanged.
	reporter(store, reportTime).WithNearby(n).Augment(scannedHosts(hostOf("10.0.0.7", true, false, 80)), "ws", "nightly", "")
	found := false
	for _, f := range store.data["ws/nightly"].Findings {
		found = found || f.Check == CheckRogueGateway
	}
	if !found {
		t.Fatal("the finding was dropped")
	}
}

func TestRunReporter_ANearbyStateThatCannotBeReadIsStatedNotHidden(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	got := reporter(store, reportTime).WithNearby(&fakeNearby{err: errors.New("disk")}).Augment(scannedHosts(hostOf("10.0.0.7", true, false, 80)), "ws", "nightly", "")
	if !strings.Contains(got, "could not be read") {
		t.Fatalf("trouble was hidden:\n%s", got)
	}
}

func TestRunReporter_AServiceFindingThatCouldNotBeJudgedIsCarried(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	cam := Finding{Device: "10.0.0.7", Port: 554, Proto: "tcp", Check: CheckRTSPNoAuth, Severity: SeverityHigh, Evidence: "w"}
	host := hostOf("10.0.0.7", true, false, 554)
	reporter(store, reportTime.Add(-24*time.Hour)).WithServiceChecks(&fakeServices{found: []Finding{cam}}).Augment(scannedHosts(host), "ws", "nightly", "")
	reporter(store, reportTime).WithServiceChecks(&fakeServices{kept: []Finding{cam}}).Augment(scannedHosts(host), "ws", "nightly", "")
	found := false
	for _, f := range store.data["ws/nightly"].Findings {
		found = found || f.Check == CheckRTSPNoAuth
	}
	if !found {
		t.Fatal("a night the camera did not answer resolved its finding")
	}
	reporter(store, reportTime.Add(24*time.Hour)).WithServiceChecks(&fakeServices{}).Augment(scannedHosts(host), "ws", "nightly", "")
	for _, f := range store.data["ws/nightly"].Findings {
		if f.Check == CheckRTSPNoAuth {
			t.Fatal("a night the stream was seen protected must resolve it")
		}
	}
}
