package netaudit

import (
	"context"
	"fmt"
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
	now   func() time.Time
}

func NewRunReporter(store BaselineStore) *RunReporter {
	return &RunReporter{store: store, now: time.Now}
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
	observed := RenderObserved(Observed{ScannedAt: at, Previous: prev.ScannedAt, Result: result, Delta: delta})
	if readErr != nil {
		observed += "\n\n> Note: the previous scan result could not be read, so no comparison was made."
	}

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
