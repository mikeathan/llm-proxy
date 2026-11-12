package netaudit

import (
	"encoding/json"
	"strings"
	"time"

	"llm-proxy/models"
)

// RecordFileName is the audit's typed record in a run's folder, beside report.md.
const RecordFileName = "audit.json"

// recordVersion is the shape of AuditRecord; a reader that does not know a version shows the Markdown report instead.
const recordVersion = 1

// noteMark starts every note paragraph the report writes.
const noteMark = "> Note:"

// AuditRecord is what the Observed part of a report states, as typed data: the same facts RenderObserved writes as Markdown, so a screen
// lays them out without parsing text. Every value is one the report already shows (counts, plain words, vetted names), never raw device text.
type AuditRecord struct {
	Version   int       `json:"version"`
	Audit     string    `json:"audit"`
	ScannedAt time.Time `json:"scanned_at"`
	// Previous is when the scan before was made; zero for a first scan.
	Previous time.Time `json:"previous_scan_at,omitzero"`
	Engine   string    `json:"engine"`
	// Scanned is how many addresses were looked at and Up how many answered.
	Scanned int `json:"scanned"`
	Up      int `json:"up"`
	// Counts are this scan's findings against the one before.
	Counts RecordCounts `json:"counts"`
	// Vulns says how far known-vulnerability matching reached.
	Vulns VulnCoverage `json:"vulns"`
	// Exposed are the remote-administration, file-sharing and database services this scan confirmed.
	Exposed []ExposedService `json:"exposed,omitempty"`
	// Window is how the network moved over the audit's window, without its open-problems list (the findings are that list); absent when none was built.
	Window *WindowDigest `json:"window,omitempty"`
	// Notes are the report's code-written notes (what could not be done and why), one string each.
	Notes []string `json:"notes,omitempty"`
	// Assessment is the model's words, kept as text: a screen shows it as Markdown through the sanctioned renderer.
	Assessment string `json:"assessment,omitempty"`
}

// RecordCounts is how many findings a scan has by kind. Open is every check finding not accepted, and BySeverity splits it.
type RecordCounts struct {
	New        int            `json:"new"`
	Changed    int            `json:"changed"`
	Resolved   int            `json:"resolved"`
	Accepted   int            `json:"accepted"`
	OpenPorts  int            `json:"open_ports"`
	Open       int            `json:"open"`
	BySeverity map[string]int `json:"by_severity,omitempty"`
}

// RecordOf builds the typed record of a scan from the same view the Markdown is rendered from. observed is that Markdown (the notes are
// lifted from it, since each is written where its cause is known) and assessment the model's text.
func RecordOf(o Observed, audit, observed, assessment string) AuditRecord {
	up := 0
	for _, h := range o.Result.Hosts {
		if h.Up {
			up++
		}
	}
	rec := AuditRecord{
		Version: recordVersion, Audit: audit, ScannedAt: o.ScannedAt, Previous: o.Previous, Engine: o.Result.Engine,
		Scanned: len(o.Result.Hosts), Up: up, Vulns: o.Vulns, Exposed: o.Exposed,
		Notes: notesOf(observed), Assessment: strings.TrimSpace(assessment),
		Counts: RecordCounts{New: len(o.Delta.New), Changed: len(o.Delta.Changed), Resolved: len(o.Delta.Resolved), Accepted: o.Accepted},
	}
	if o.Window != nil { // the open problems are the findings' own list; their service and version text came from devices
		w := *o.Window
		w.Open, w.OpenMore = nil, 0
		rec.Window = &w
	}
	for _, f := range o.Result.Findings {
		if f.Check == CheckOpenPort {
			rec.Counts.OpenPorts++
			continue
		}
		rec.Counts.Open++
		if rec.Counts.BySeverity == nil {
			rec.Counts.BySeverity = map[string]int{}
		}
		rec.Counts.BySeverity[f.Severity]++
	}
	return rec
}

// notesOf lifts the note paragraphs out of the Markdown, flattened to one line each.
func notesOf(observed string) []string {
	var out []string
	for _, para := range strings.Split(observed, "\n\n") {
		if p := strings.TrimSpace(para); strings.HasPrefix(p, noteMark) {
			out = append(out, strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(strings.TrimPrefix(p, noteMark), "\n> ", " "), "\n", " ")))
		}
	}
	return out
}

// File is the record as the file kept beside the report, or a zero ReportFile when it cannot be written.
func (r AuditRecord) File() (models.ReportFile, bool) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return models.ReportFile{}, false
	}
	return models.ReportFile{Name: RecordFileName, Content: string(data) + "\n"}, true
}
