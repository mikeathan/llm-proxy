package netaudit

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"llm-proxy/models"
)

var digestNow = time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)

func digestInv() models.DeviceInventory {
	old := digestNow.AddDate(0, -3, 0)
	return models.DeviceInventory{Baselined: true, Devices: []models.NetworkDevice{
		{MAC: "aa:00:00:00:00:01", IP: "192.168.1.1", Name: "Router", Approved: true, FirstSeen: old, LastSeen: digestNow},
		{MAC: "aa:00:00:00:00:02", IP: "192.168.1.5", Approved: true, FirstSeen: digestNow.Add(-48 * time.Hour), LastSeen: digestNow},
		{MAC: "aa:00:00:00:00:03", IP: "192.168.1.6", Name: "Old TV", Approved: true, FirstSeen: old, LastSeen: digestNow.AddDate(0, 0, -9)},
	}}
}

func day(offset int, findings, serious int, audited bool) models.NetworkDay {
	return models.NetworkDay{Date: digestNow.AddDate(0, 0, -offset).Format(historyDateLayout), Findings: findings, Serious: serious, Audited: audited}
}

func TestDigestMessage_CountsTheWeekInFixedWordsWithoutNamesOrAddresses(t *testing.T) {
	series := []models.NetworkDay{day(7, 6, 2, true), day(3, 5, 1, true), day(0, 4, 1, true)}
	got := DigestMessage(digestInv(), series, digestNow)
	for _, want := range []string{"3 devices known", "1 new this week", "1 not seen this week", "1 without a name", "Open findings: 4", "was 6 a week ago", "1 serious"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	for _, leak := range []string{"Router", "Old TV", "192.168.1", "aa:00"} {
		if strings.Contains(got, leak) {
			t.Errorf("the summary must hold counts only, found %q in %q", leak, got)
		}
	}
}

func TestDigestMessage_SaysSoWhenNoAuditHasRunAndOmitsTheComparisonItCannotMake(t *testing.T) {
	if got := DigestMessage(digestInv(), nil, digestNow); !strings.Contains(got, "No network audit has run yet") || strings.Contains(got, "a week ago") {
		t.Errorf("message = %q", got)
	}
	if got := DigestMessage(digestInv(), []models.NetworkDay{day(0, 4, 0, true)}, digestNow); strings.Contains(got, "a week ago") || !strings.Contains(got, "Open findings: 4") {
		t.Errorf("without an older day there is nothing to compare: %q", got)
	}
}

type digestHist struct {
	last    time.Time
	series  []models.NetworkDay
	marked  []time.Time
	readErr error
}

func (d *digestHist) Series(int) ([]models.NetworkDay, error) { return d.series, d.readErr }
func (d *digestHist) LastDigest() (time.Time, error)          { return d.last, d.readErr }
func (d *digestHist) MarkDigest(at time.Time) error {
	d.marked = append(d.marked, at)
	d.last = at
	return nil
}

type digestRig struct {
	enabled bool
	hist    *digestHist
	sent    []string
	sendErr error
	errs    []error
	now     time.Time
}

func (r *digestRig) digest() *WeeklyDigest {
	return NewWeeklyDigest(DigestDeps{
		Enabled: func() bool { return r.enabled },
		View:    func() (models.DeviceInventory, error) { return digestInv(), nil },
		History: r.hist,
		Notify:  func(_ context.Context, m string) error { r.sent = append(r.sent, m); return r.sendErr },
		Now:     func() time.Time { return r.now },
		OnError: func(err error) { r.errs = append(r.errs, err) },
	})
}

func newDigestRig() *digestRig { return &digestRig{enabled: true, hist: &digestHist{}, now: digestNow} }

func TestWeeklyDigest_IsOffUntilTurnedOn(t *testing.T) {
	r := newDigestRig()
	r.enabled = false
	r.digest().Tick(context.Background())
	if len(r.sent) != 0 {
		t.Error("a switched-off summary must send nothing")
	}
}

func TestWeeklyDigest_SendsOnceThenWaitsAWeek(t *testing.T) {
	r := newDigestRig()
	d := r.digest()
	d.Tick(context.Background())
	if len(r.sent) != 1 || len(r.hist.marked) != 1 {
		t.Fatalf("sent %d, marked %d", len(r.sent), len(r.hist.marked))
	}
	r.now = r.now.Add(6*24*time.Hour + 23*time.Hour)
	d.Tick(context.Background())
	if len(r.sent) != 1 {
		t.Fatalf("sent again inside the week: %d", len(r.sent))
	}
	r.now = r.now.Add(2 * time.Hour)
	d.Tick(context.Background())
	if len(r.sent) != 2 {
		t.Errorf("sent %d, want 2 after the week", len(r.sent))
	}
}

func TestWeeklyDigest_AFailedSendIsNotMarkedAndIsReportedOncePerCause(t *testing.T) {
	r := newDigestRig()
	r.sendErr = errors.New("no connector is set up to send alerts")
	d := r.digest()
	for range 3 {
		d.Tick(context.Background())
	}
	if len(r.hist.marked) != 0 {
		t.Error("an unsent summary must stay due")
	}
	if len(r.errs) != 1 {
		t.Errorf("errors = %d, want the same cause reported once", len(r.errs))
	}
	r.sendErr = nil
	d.Tick(context.Background())
	if len(r.sent) != 4 || len(r.hist.marked) != 1 {
		t.Errorf("after the connector exists it is sent: sent %d, marked %d", len(r.sent), len(r.hist.marked))
	}
}
