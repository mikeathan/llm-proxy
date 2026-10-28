package netaudit

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"llm-proxy/models"
)

// BaselineStore keeps an automation's latest scan result. *persistence.WorkspaceManager satisfies it.
type BaselineStore interface {
	ReadBaseline(workspaceID, automation string) (models.ScanBaseline, error)
	WriteBaseline(workspaceID, automation string, b models.ScanBaseline) error
}

// RunReporter turns the scans an automation run made into the code-rendered part of its report, and keeps the result
// as the baseline for the next run. It never reads the model's words: it works from what the scan tool collected.
type RunReporter struct {
	store  BaselineStore
	acks   AckStore         // optional: findings the operator accepted
	fixes  FixStore         // optional: findings the operator marked as fixed
	vulns  VulnSource       // optional: the vulnerability data known_vulns findings are matched against
	expect ExpectationStore // optional: what the operator says each device should offer
	now    func() time.Time
}

func NewRunReporter(store BaselineStore) *RunReporter {
	return &RunReporter{store: store, now: time.Now}
}

// WithAcks makes the report leave out findings the operator has accepted (counting them instead) and keeps the
// acceptances up to date. Without it every finding is reported.
func (r *RunReporter) WithAcks(acks AckStore) *RunReporter {
	r.acks = acks
	return r
}

// WithFixes makes the reporter retire the fix records of findings a scan no longer shows, and keep when each was marked
// with the scan so the screen can say the fix is confirmed. A record never changes what is listed or compared.
func (r *RunReporter) WithFixes(fixes FixStore) *RunReporter {
	r.fixes = fixes
	return r
}

// WithExpectations makes the reporter compare each device that has an expectation with it. The differences join the scan's
// findings, so they are compared with the last scan, accepted and kept like any other. Without it nothing is compared.
func (r *RunReporter) WithExpectations(store ExpectationStore) *RunReporter {
	r.expect = store
	return r
}

// Summary is the connector-safe account of the run's scan (counts only), or "" when the run made no scan.
func (r *RunReporter) Summary(ctx context.Context) string {
	if collector := CollectorFrom(ctx); collector != nil {
		return collector.Summary()
	}
	return ""
}

// Begin makes the run's context collect scan results. It is cheap, so every automation run gets it.
func (r *RunReporter) Begin(ctx context.Context) context.Context {
	ctx, _ = WithCollector(ctx)
	return ctx
}

// Augment returns the report with the observed facts first and the model's assessment after, labelled as such. A run
// that made no scan gets its report back unchanged. The baseline is replaced only by a report that was built, so a run
// that scanned nothing leaves the last good one in place.
func (r *RunReporter) Augment(ctx context.Context, workspaceID, automation, report string) string {
	collector := CollectorFrom(ctx)
	if collector == nil {
		return report
	}
	result, ok := collector.Merged()
	if !ok {
		return report
	}
	cov := r.addKnownVulns(ctx, &result)
	expectNote := r.addExpectations(workspaceID, &result)
	at := r.now()
	prev, readErr := r.store.ReadBaseline(workspaceID, automation)

	scanned := scannedDevices(result.Hosts)
	delta := Diff(restrictTo(prev.Findings, scanned), result.Findings)
	shown, accepted, ackNote := r.applyAcks(workspaceID, result, delta, at)
	view := Observed{ScannedAt: at, Previous: prev.ScannedAt, Result: shown.result, Delta: shown.delta, Accepted: accepted, Vulns: cov}
	observed := RenderObserved(view)
	collector.SetSummary(SummaryOf(Observed{ScannedAt: at, Previous: prev.ScannedAt, Result: result, Delta: shown.delta, Accepted: accepted, Vulns: cov}, automation))
	if readErr != nil {
		observed += "\n\n> Note: the previous scan result could not be read, so no comparison was made."
	}
	observed += ackNote + expectNote

	next := models.ScanBaseline{
		ScannedAt: at, Engine: result.Engine, Findings: append(withoutDevices(prev.Findings, scanned), result.Findings...),
		NewKeys: keysOf(delta.New), Resolved: delta.Resolved,
		ChangedFrom: changedFrom(delta.Changed),
		// A baseline that could not be read says nothing about whether one existed, so it is not called a first scan.
		FirstScan: readErr == nil && prev.ScannedAt.IsZero(),
	}
	next.Unscanned, next.DevicesUp = carryOver(prev, result.Hosts)
	fixedAt, fixNote := r.confirmFixes(workspaceID, delta.Resolved)
	next.FixedAt = fixedAt
	observed += fixNote
	if err := r.store.WriteBaseline(workspaceID, automation, next); err != nil {
		observed += fmt.Sprintf("\n\n> Note: this scan could not be saved for next time (%v).", err)
	}
	return observed + "\n\n---\n\n" + AssessmentHeading + "\n\n" + report
}

// changedFrom maps each changed finding's key to the version it had before.
func changedFrom(changes []Change) map[string]string {
	if len(changes) == 0 {
		return nil
	}
	out := make(map[string]string, len(changes))
	for _, c := range changes {
		out[c.After.Key()] = c.Before.Version
	}
	return out
}

// unscannedDevices lists the addresses the engine gave up on.
func unscannedDevices(hosts []Host) []string {
	var out []string
	for _, h := range hosts {
		if h.Unscanned {
			out = append(out, h.Addr.String())
		}
	}
	return out
}

// carryOver is the unscanned list and the count of answering addresses after this run. A run that looked at only some
// devices (a rescan, or an audit aimed at one range) does not forget the others: a device flagged earlier and not
// looked at now stays flagged, and a device that earlier had findings or was flagged counts as answering.  A run over
// every device changes nothing here, because every earlier device is among this run's hosts.
func carryOver(prev models.ScanBaseline, hosts []Host) ([]string, int) {
	seen := make(map[string]bool, len(hosts))
	for _, h := range hosts {
		seen[h.Addr.String()] = true
	}
	unscanned := unscannedDevices(hosts)
	up := devicesUp(hosts)
	kept := map[string]bool{}
	for _, d := range prev.Unscanned {
		if !seen[d] && !kept[d] {
			unscanned = append(unscanned, d)
			kept[d] = true
		}
	}
	for _, f := range prev.Findings {
		if !seen[f.Device] && !kept[f.Device] {
			kept[f.Device] = true
		}
	}
	return unscanned, up + len(kept)
}

func devicesUp(hosts []Host) int {
	n := 0
	for _, h := range hosts {
		if h.Up {
			n++
		}
	}
	return n
}

func keysOf(findings []Finding) []string {
	var keys []string
	for _, f := range findings {
		keys = append(keys, f.Key())
	}
	return keys
}

// shownScan is the part of a scan the report lists: what no active acceptance covers.
type shownScan struct {
	result Result
	delta  Delta
}

// applyAcks removes the findings the operator accepted from what the report lists, returns how many that was, and saves
// the refreshed acceptances. A problem reading or saving them is stated in the note, and nothing is hidden when they
// cannot be read.
func (r *RunReporter) applyAcks(workspaceID string, result Result, delta Delta, at time.Time) (shownScan, int, string) {
	shown := shownScan{result: result, delta: delta}
	if r.acks == nil {
		return shown, 0, ""
	}
	acks, err := r.acks.ReadAcks(workspaceID)
	if err != nil {
		return shown, 0, "\n\n> Note: the accepted findings could not be read, so every finding is listed."
	}
	applied := ApplyAcks(result.Findings, acks, at)
	covered := make(map[string]bool, len(applied.Accepted))
	for _, f := range applied.Accepted {
		covered[f.Key()] = true
	}
	shown.result.Findings = applied.Visible
	shown.delta.New = withoutKeys(delta.New, covered)
	shown.delta.Changed = nil
	for _, c := range delta.Changed {
		if !covered[c.After.Key()] {
			shown.delta.Changed = append(shown.delta.Changed, c)
		}
	}
	note := ""
	if !reflect.DeepEqual(acks, applied.Acks) {
		if err := r.acks.WriteAcks(workspaceID, applied.Acks); err != nil {
			note = fmt.Sprintf("\n\n> Note: the accepted findings could not be updated (%v).", err)
		}
	}
	return shown, len(applied.Accepted), note
}

// confirmFixes retires the fix records of the findings this scan no longer shows and returns when each was marked. A
// problem reading or saving the records is stated in the note and changes nothing else.
func (r *RunReporter) confirmFixes(workspaceID string, resolved []Finding) (map[string]time.Time, string) {
	if r.fixes == nil || len(resolved) == 0 {
		return nil, ""
	}
	records, err := r.fixes.ReadFixes(workspaceID)
	if err != nil {
		return nil, "\n\n> Note: the fix records could not be read, so no fix could be confirmed."
	}
	kept, confirmed := ConfirmFixes(records, resolved)
	if len(confirmed) == 0 {
		return nil, ""
	}
	if err := r.fixes.WriteFixes(workspaceID, kept); err != nil {
		return confirmed, fmt.Sprintf("\n\n> Note: the fix records could not be updated (%v).", err)
	}
	return confirmed, ""
}

func withoutKeys(findings []Finding, drop map[string]bool) []Finding {
	var out []Finding
	for _, f := range findings {
		if !drop[f.Key()] {
			out = append(out, f)
		}
	}
	return out
}

// scannedDevices is the set of addresses this run looked at to the end, up or down.
func scannedDevices(hosts []Host) map[string]bool {
	out := make(map[string]bool, len(hosts))
	for _, h := range hosts {
		if !h.Unscanned { // an address the engine gave up on has not been looked at: its earlier findings stand
			out[h.Addr.String()] = true
		}
	}
	return out
}

// restrictTo keeps the findings of the devices in the set: an address this run did not scan has not "resolved".
func restrictTo(findings []Finding, devices map[string]bool) []Finding {
	var out []Finding
	for _, f := range findings {
		if devices[f.Device] {
			out = append(out, f)
		}
	}
	return out
}

// withoutDevices drops the findings of the devices in the set, to be replaced by this run's.
func withoutDevices(findings []Finding, devices map[string]bool) []Finding {
	var out []Finding
	for _, f := range findings {
		if !devices[f.Device] {
			out = append(out, f)
		}
	}
	return out
}

// addExpectations appends what differs from the operator's expectations to the result, before the comparison with the last
// scan. A store that cannot be read is stated in the report and never stops the scan from being reported.
func (r *RunReporter) addExpectations(workspaceID string, result *Result) string {
	if r.expect == nil {
		return ""
	}
	items, err := r.expect.ReadExpectations(workspaceID)
	if err != nil {
		return fmt.Sprintf("\n\n> Note: the expected ports could not be read, so they were not compared (%v).", err)
	}
	result.Findings = append(result.Findings, ExpectationFindings(items, result.Hosts)...)
	return ""
}
