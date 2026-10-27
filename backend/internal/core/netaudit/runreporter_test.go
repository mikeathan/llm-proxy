package netaudit

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"

	"llm-proxy/models"
)

type memBaselines struct {
	data     map[string]models.ScanBaseline
	readErr  error
	writeErr error
}

func (m *memBaselines) key(ws, a string) string { return ws + "/" + a }
func (m *memBaselines) ReadBaseline(ws, a string) (models.ScanBaseline, error) {
	return m.data[m.key(ws, a)], m.readErr
}
func (m *memBaselines) WriteBaseline(ws, a string, b models.ScanBaseline) error {
	if m.writeErr != nil {
		return m.writeErr
	}
	m.data[m.key(ws, a)] = b
	return nil
}

func reporter(store *memBaselines, at time.Time) *RunReporter {
	r := NewRunReporter(store)
	r.now = func() time.Time { return at }
	return r
}

func scanned(findings ...Finding) context.Context {
	ctx, c := WithCollector(context.Background())
	c.Add(Result{Engine: "nmap", Hosts: hostsWith(findings...), Findings: findings})
	return ctx
}

func TestRunReporter_ARunWithoutScansGetsItsReportBackUntouched(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	r := reporter(store, reportTime)
	for _, ctx := range []context.Context{context.Background(), r.Begin(context.Background())} {
		if got := r.Augment(ctx, "ws", "nightly", "the model's report"); got != "the model's report" {
			t.Errorf("got %q", got)
		}
	}
	if len(store.data) != 0 {
		t.Error("a run that scanned nothing must not write a baseline")
	}
}

func TestRunReporter_ObservedComesFirstAndTheModelIsLabelled(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	got := reporter(store, reportTime).Augment(scanned(finding("10.0.0.1", 22, "")), "ws", "nightly", "all fine, nothing to worry about")
	observed, assessment, found := strings.Cut(got, AssessmentHeading)
	if !found || !strings.HasPrefix(observed, ObservedHeading) || !strings.Contains(assessment, "all fine, nothing to worry about") {
		t.Errorf("report:\n%s", got)
	}
	if strings.Contains(observed, "all fine") {
		t.Error("the model's words must not appear in the observed section")
	}
}

func TestRunReporter_SecondRunIsComparedWithTheFirst(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	r := reporter(store, reportTime)
	r.Augment(scanned(finding("10.0.0.1", 22, ""), finding("10.0.0.1", 80, "")), "ws", "nightly", "")

	r.now = func() time.Time { return reportTime.Add(24 * time.Hour) }
	got := r.Augment(scanned(finding("10.0.0.1", 22, ""), finding("10.0.0.1", 3306, "")), "ws", "nightly", "")
	if !strings.Contains(got, "1 new, 1 resolved, 0 changed, 1 unchanged") {
		t.Errorf("second run:\n%s", got)
	}
	b := store.data["ws/nightly"]
	if len(b.Findings) != 2 || !b.ScannedAt.Equal(reportTime.Add(24*time.Hour)) {
		t.Errorf("baseline = %+v (must be the latest scan only)", b)
	}
	if len(b.NewKeys) != 1 || b.NewKeys[0] != "10.0.0.1|tcp|3306|open_port" || len(b.Resolved) != 1 || b.Resolved[0].Port != 80 {
		t.Errorf("the baseline must remember what changed: new %v, resolved %+v", b.NewKeys, b.Resolved)
	}
}

func TestRunReporter_AnAddressNotScannedThisTimeIsNotReportedResolved(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	r := reporter(store, reportTime)
	r.Augment(scanned(finding("10.0.0.1", 22, ""), finding("10.0.0.2", 445, "")), "ws", "nightly", "")

	// The second run only looks at 10.0.0.1.
	got := r.Augment(scanned(finding("10.0.0.1", 22, "")), "ws", "nightly", "")
	if !strings.Contains(got, "0 new, 0 resolved") {
		t.Errorf("an address that was not scanned has not resolved:\n%s", got)
	}
	if len(store.data["ws/nightly"].Findings) != 2 {
		t.Errorf("the unscanned address's earlier findings must be carried forward: %+v", store.data["ws/nightly"])
	}
}

func TestRunReporter_AHostNowDownResolvesItsPorts(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	r := reporter(store, reportTime)
	r.Augment(scanned(finding("10.0.0.2", 445, "")), "ws", "nightly", "")

	ctx, c := WithCollector(context.Background())
	c.Add(Result{Engine: "nmap", Hosts: []Host{{Addr: netip.MustParseAddr("10.0.0.2"), Up: false}}})
	got := r.Augment(ctx, "ws", "nightly", "")
	if !strings.Contains(got, "0 new, 1 resolved") || !strings.Contains(got, "0 up, 1 down") {
		t.Errorf("a host that no longer answers:\n%s", got)
	}
}

func TestRunReporter_StorageProblemsAreStatedNotHidden(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}, readErr: errors.New("corrupt"), writeErr: errors.New("disk full")}
	got := reporter(store, reportTime).Augment(scanned(finding("10.0.0.1", 22, "")), "ws", "nightly", "")
	for _, want := range []string{"could not be read", "could not be saved"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
}

func TestRunReporter_AHostTheScanGaveUpOnKeepsItsEarlierFindingsAndIsFlagged(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	r := reporter(store, reportTime)
	r.Augment(scanned(finding("10.0.0.1", 22, ""), finding("10.0.0.2", 445, "")), "ws", "nightly", "")

	ctx, c := WithCollector(context.Background())
	c.Add(Result{Engine: "nmap", Hosts: []Host{
		{Addr: netip.MustParseAddr("10.0.0.1"), Up: true, Ports: []Port{{Number: 22, Proto: "tcp", State: "open"}}},
		{Addr: netip.MustParseAddr("10.0.0.2"), Up: true, Unscanned: true},
	}})
	got := r.Augment(ctx, "ws", "nightly", "")
	for _, want := range []string{"**not scanned to completion**", "10.0.0.2", "0 new, 0 resolved"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
	if len(store.data["ws/nightly"].Findings) != 2 {
		t.Errorf("the abandoned host's earlier finding must be carried forward: %+v", store.data["ws/nightly"].Findings)
	}
}

func TestRunReporter_TheBaselineRecordsWhatChangedWhatWasUnscannedAndHowManyAnswered(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	r := reporter(store, reportTime)
	r.Augment(scanned(finding("10.0.0.1", 22, "9.6p1"), finding("10.0.0.2", 445, "")), "ws", "nightly", "")

	ctx, c := WithCollector(context.Background())
	c.Add(Result{Engine: "nmap", Hosts: []Host{
		{Addr: netip.MustParseAddr("10.0.0.1"), Up: true, Ports: []Port{{Number: 22, Proto: "tcp", State: "open", Version: "9.7p1"}}},
		{Addr: netip.MustParseAddr("10.0.0.2"), Up: true, Unscanned: true},
		{Addr: netip.MustParseAddr("10.0.0.3"), Up: false},
	}})
	r.Augment(ctx, "ws", "nightly", "")

	b := store.data["ws/nightly"]
	if was, ok := b.ChangedFrom[finding("10.0.0.1", 22, "").Key()]; !ok || was != "9.6p1" {
		t.Errorf("ChangedFrom = %v, want the ssh finding's earlier version", b.ChangedFrom)
	}
	if len(b.Unscanned) != 1 || b.Unscanned[0] != "10.0.0.2" {
		t.Errorf("Unscanned = %v", b.Unscanned)
	}
	if b.DevicesUp != 2 {
		t.Errorf("DevicesUp = %d, want 2", b.DevicesUp)
	}
}

func TestRunReporter_ARescanOfOneDeviceKeepsWhatIsKnownAboutTheOthers(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	r := reporter(store, reportTime)
	ctx, c := WithCollector(context.Background())
	c.Add(Result{Engine: "nmap", Hosts: []Host{
		{Addr: netip.MustParseAddr("10.0.0.1"), Up: true, Ports: []Port{{Number: 22, Proto: "tcp", State: "open"}}},
		{Addr: netip.MustParseAddr("10.0.0.2"), Up: true, Ports: []Port{{Number: 445, Proto: "tcp", State: "open"}}},
		{Addr: netip.MustParseAddr("10.0.0.3"), Up: true, Unscanned: true},
	}, Findings: []Finding{finding("10.0.0.1", 22, ""), finding("10.0.0.2", 445, "")}})
	r.Augment(ctx, "ws", "nightly", "")

	rescan, rc := WithCollector(context.Background())
	rc.Add(Result{Engine: "nmap", Hosts: []Host{{Addr: netip.MustParseAddr("10.0.0.1"), Up: true}}})
	r.Augment(rescan, "ws", "nightly", "")

	b := store.data["ws/nightly"]
	if len(b.Unscanned) != 1 || b.Unscanned[0] != "10.0.0.3" {
		t.Errorf("a device this run did not look at stays flagged: Unscanned = %v", b.Unscanned)
	}
	if b.DevicesUp != 3 {
		t.Errorf("DevicesUp = %d, want 3 (the rescanned device, the one with findings, the flagged one)", b.DevicesUp)
	}
}

type memAcks struct {
	acks     []models.ScanAck
	readErr  error
	writeErr error
	writes   int
}

func (m *memAcks) ReadAcks(string) ([]models.ScanAck, error) { return m.acks, m.readErr }
func (m *memAcks) WriteAcks(_ string, a []models.ScanAck) error {
	m.writes++
	if m.writeErr != nil {
		return m.writeErr
	}
	m.acks = a
	return nil
}

func TestRunReporter_AcceptedFindingsAreCountedNotListedAndStayInTheBaseline(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	ssh, telnet := finding("10.0.0.1", 22, "OpenSSH 9"), finding("10.0.0.1", 23, "")
	ack, _ := NewAck(ssh, "jump host", 30, reportTime.Add(-time.Hour))
	acks := &memAcks{acks: []models.ScanAck{ack}}
	r := reporter(store, reportTime).WithAcks(acks)

	got := r.Augment(scanned(ssh, telnet), "ws", "nightly", "")
	if !strings.Contains(got, "1 finding(s) in this scan are accepted by the operator") {
		t.Errorf("the accepted count is stated:\n%s", got)
	}
	if strings.Contains(got, "22/tcp") || !strings.Contains(got, "23/tcp") {
		t.Errorf("the accepted port must not be listed, the other must:\n%s", got)
	}
	if len(store.data["ws/nightly"].Findings) != 2 {
		t.Errorf("the baseline keeps every finding, accepted or not: %+v", store.data["ws/nightly"].Findings)
	}
	if acks.writes != 1 || !acks.acks[0].LastSeen.Equal(reportTime) {
		t.Errorf("a seen acceptance is refreshed and saved: %+v (writes %d)", acks.acks, acks.writes)
	}
}

func TestRunReporter_AChangedVersionOrAnExpiredAcceptanceComesBackIntoTheReport(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	r := reporter(store, reportTime)
	r.Augment(scanned(finding("10.0.0.1", 22, "OpenSSH 8")), "ws", "nightly", "") // the baseline

	ack, _ := NewAck(finding("10.0.0.1", 22, "OpenSSH 8"), "", 30, reportTime.Add(-time.Hour))
	r = reporter(store, reportTime.Add(24*time.Hour)).WithAcks(&memAcks{acks: []models.ScanAck{ack}})
	got := r.Augment(scanned(finding("10.0.0.1", 22, "OpenSSH 9")), "ws", "nightly", "")
	if !strings.Contains(got, "### Changed") || strings.Contains(got, "accepted by the operator") {
		t.Errorf("an upgrade after the acceptance is news again:\n%s", got)
	}

	expired, _ := NewAck(finding("10.0.0.1", 22, "OpenSSH 9"), "", 1, reportTime.Add(-72*time.Hour))
	r = reporter(&memBaselines{data: map[string]models.ScanBaseline{}}, reportTime).WithAcks(&memAcks{acks: []models.ScanAck{expired}})
	got = r.Augment(scanned(finding("10.0.0.1", 22, "OpenSSH 9")), "ws", "nightly", "")
	if !strings.Contains(got, "22/tcp") || strings.Contains(got, "accepted by the operator") {
		t.Errorf("an expired acceptance does not hide anything:\n%s", got)
	}
}

func TestRunReporter_AcknowledgementTroubleIsStatedAndNeverHidesFindings(t *testing.T) {
	f := finding("10.0.0.1", 22, "")
	got := reporter(&memBaselines{data: map[string]models.ScanBaseline{}}, reportTime).WithAcks(&memAcks{readErr: errors.New("corrupt")}).
		Augment(scanned(f), "ws", "nightly", "")
	if !strings.Contains(got, "could not be read, so every finding is listed") || !strings.Contains(got, "22/tcp") {
		t.Errorf("unreadable acceptances must not hide findings:\n%s", got)
	}
}

func TestRunReporter_TheSummaryIsSetAndHonoursAcceptedFindings(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	ssh, telnet := finding("10.0.0.1", 22, ""), finding("10.0.0.1", 23, "")
	ack, _ := NewAck(ssh, "", 30, reportTime.Add(-time.Hour))
	r := reporter(store, reportTime).WithAcks(&memAcks{acks: []models.ScanAck{ack}})

	ctx := r.Begin(context.Background())
	if r.Summary(ctx) != "" {
		t.Error("no summary before a scan")
	}
	collector := CollectorFrom(ctx)
	collector.Add(Result{Engine: "nmap", Hosts: hostsWith(ssh, telnet), Findings: []Finding{ssh, telnet}})
	r.Augment(ctx, "ws", "nightly", "model text")
	got := r.Summary(ctx)
	if !strings.Contains(got, "1 accepted") || !strings.Contains(got, "first scan, 2 open port(s)") {
		t.Errorf("summary = %q", got)
	}
	if r.Summary(context.Background()) != "" {
		t.Error("a context without a collector has no summary")
	}
}
