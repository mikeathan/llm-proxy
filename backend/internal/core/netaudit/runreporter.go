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
	store BaselineStore
	acks  AckStore // optional: findings the operator accepted
	now   func() time.Time
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
	at := r.now()
	prev, readErr := r.store.ReadBaseline(workspaceID, automation)

	scanned := scannedDevices(result.Hosts)
	delta := Diff(restrictTo(prev.Findings, scanned), result.Findings)
	shown, accepted, ackNote := r.applyAcks(workspaceID, result, delta, at)
	view := Observed{ScannedAt: at, Previous: prev.ScannedAt, Result: shown.result, Delta: shown.delta, Accepted: accepted}
	observed := RenderObserved(view)
	collector.SetSummary(SummaryOf(Observed{ScannedAt: at, Previous: prev.ScannedAt, Result: result, Delta: shown.delta, Accepted: accepted}, automation))
	if readErr != nil {
		observed += "\n\n> Note: the previous scan result could not be read, so no comparison was made."
	}
	observed += ackNote

	next := models.ScanBaseline{
		ScannedAt: at, Engine: result.Engine, Findings: append(withoutDevices(prev.Findings, scanned), result.Findings...),
		NewKeys: keysOf(delta.New), Resolved: delta.Resolved,
	}
	if err := r.store.WriteBaseline(workspaceID, automation, next); err != nil {
		observed += fmt.Sprintf("\n\n> Note: this scan could not be saved for next time (%v).", err)
	}
	return observed + "\n\n---\n\n" + AssessmentHeading + "\n\n" + report
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
