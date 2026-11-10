package netaudit

import (
	"strings"
	"testing"
	"time"

	"llm-proxy/models"
)

type fakeFootprint struct {
	findings []Finding
	checked  map[string]bool
}

func (f *fakeFootprint) Findings(time.Time) ([]Finding, map[string]bool) {
	return f.findings, f.checked
}

func spfFinding() Finding {
	return Finding{Device: "domain:example.com", Check: "mail_spf_missing", Severity: "medium", Evidence: "The domain publishes no SPF record, so anyone can send mail that claims to be from it."}
}

func runWith(store *memBaselines, fp *fakeFootprint, at time.Time) string {
	return reporter(store, at).WithFootprint(fp).Augment(scannedHosts(smbHost("10.0.0.1", true)), "ws", "nightly", "")
}

// The same footprint result on two nights is the same finding: not new again, and never listed twice.
func TestRunReporter_AFootprintFindingIsNotNewEveryNight(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	fp := &fakeFootprint{findings: []Finding{spfFinding()}, checked: map[string]bool{"domain:example.com": true}}
	runWith(store, fp, reportTime.Add(-24*time.Hour))
	runWith(store, fp, reportTime)
	b := store.data["ws/nightly"]
	count := 0
	for _, f := range b.Findings {
		if f.Device == "domain:example.com" {
			count++
		}
	}
	if count != 1 || len(b.NewKeys) != 0 {
		t.Errorf("%d copies, new keys %v", count, b.NewKeys)
	}
}

// A watch that was not looked at fully and recently is carried, not resolved: a look that was not made says nothing about the
// problem being gone.
func TestRunReporter_AWatchThatWasNotLookedAtKeepsItsFindingAndResolvesNothing(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	found := &fakeFootprint{findings: []Finding{spfFinding()}, checked: map[string]bool{"domain:example.com": true}}
	runWith(store, found, reportTime.Add(-24*time.Hour))
	runWith(store, &fakeFootprint{checked: map[string]bool{}}, reportTime)
	b := store.data["ws/nightly"]
	kept := false
	for _, f := range b.Findings {
		kept = kept || f.Device == "domain:example.com"
	}
	if !kept || len(b.Resolved) != 0 {
		t.Errorf("kept=%v resolved=%v", kept, b.Resolved)
	}
}

// When the watch is looked at in full and the problem is gone, it is resolved like any other finding.
func TestRunReporter_AFootprintFindingThatIsGoneIsResolved(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	runWith(store, &fakeFootprint{findings: []Finding{spfFinding()}, checked: map[string]bool{"domain:example.com": true}}, reportTime.Add(-24*time.Hour))
	runWith(store, &fakeFootprint{checked: map[string]bool{"domain:example.com": true}}, reportTime)
	b := store.data["ws/nightly"]
	if len(b.Resolved) != 1 || !strings.HasPrefix(b.Resolved[0].Device, "domain:") {
		t.Errorf("resolved: %+v", b.Resolved)
	}
}
