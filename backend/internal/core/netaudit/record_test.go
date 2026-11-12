package netaudit

import (
	"encoding/json"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestRecordOf_StatesTheSameFactsAsTheMarkdown(t *testing.T) {
	at := time.Date(2026, 11, 12, 2, 0, 0, 0, time.UTC)
	o := Observed{
		ScannedAt: at, Previous: at.Add(-24 * time.Hour), Accepted: 2,
		Result: Result{Engine: "nmap", Hosts: []Host{{Addr: netip.MustParseAddr("10.0.0.1"), Up: true}, {Addr: netip.MustParseAddr("10.0.0.2")}},
			Findings: []Finding{
				{Device: "10.0.0.1", Port: 22, Proto: "tcp", Check: CheckOpenPort, Severity: SeverityInfo},
				{Device: HostDevice, Check: CheckHostFilesModified, Severity: SeverityHigh},
				{Device: HostDevice, Check: HostFileCheck("cron"), Severity: SeverityMedium},
				{Device: HostDevice, Check: HostFileCheck("sudoers"), Severity: SeverityMedium},
			}},
		Delta:   Delta{New: []Finding{{}}, Resolved: []Finding{{}, {}}},
		Exposed: []ExposedService{{Device: "10.0.0.1", Port: 22, Proto: "tcp", Service: "ssh", Kind: "remote administration"}},
		Vulns:   VulnCoverage{Enabled: true, Total: 5, Checked: 4},
	}
	observed := "Scanned 2 address(es).\n\n> Note: one thing could not be done\n> and here is why.\n\n### New\n\nx\n\n> Note: second"
	rec := RecordOf(o, "network-audit", observed, "  1. Look at the file.  ")

	if rec.Version != 1 || rec.Audit != "network-audit" || rec.Scanned != 2 || rec.Up != 1 || rec.Engine != "nmap" {
		t.Errorf("header = %+v", rec)
	}
	c := rec.Counts
	if c.New != 1 || c.Resolved != 2 || c.Accepted != 2 || c.OpenPorts != 1 || c.Open != 3 || c.BySeverity["high"] != 1 || c.BySeverity["medium"] != 2 {
		t.Errorf("counts = %+v", c)
	}
	if len(rec.Notes) != 2 || rec.Notes[0] != "one thing could not be done and here is why." {
		t.Errorf("notes = %q", rec.Notes)
	}
	if rec.Assessment != "1. Look at the file." {
		t.Errorf("assessment = %q", rec.Assessment)
	}
}

func TestAuditRecord_IsAFileThatReadsBackWithoutLosingAnything(t *testing.T) {
	at := time.Date(2026, 11, 12, 2, 0, 0, 0, time.UTC)
	rec := RecordOf(Observed{ScannedAt: at, Exposed: []ExposedService{{Device: "10.0.0.1", Port: 445, Proto: "tcp", Service: "smb", Kind: "file sharing"}}, Vulns: VulnCoverage{Enabled: true}}, "a", "", "")
	f, ok := rec.File()
	if !ok || f.Name != RecordFileName || !strings.HasSuffix(f.Content, "\n") {
		t.Fatalf("file = %+v, %v", f, ok)
	}
	var back AuditRecord
	if err := json.Unmarshal([]byte(f.Content), &back); err != nil {
		t.Fatal(err)
	}
	if !back.ScannedAt.Equal(at) || len(back.Exposed) != 1 || back.Exposed[0].Service != "smb" || !back.Vulns.Enabled {
		t.Errorf("read back %+v", back)
	}
	if strings.Contains(f.Content, `"previous_scan_at"`) {
		t.Error("a first scan has no previous time, so the field is left out")
	}
}

func TestRecordOf_LeavesDeviceTextOutOfTheWindow(t *testing.T) {
	o := Observed{ScannedAt: time.Now(), Window: &WindowDigest{Window: "last 24 hours", Open: []DigestOpen{{Device: "10.0.0.1", Check: "x", Severity: "high", Service: "text a device sent", Version: "9"}}, OpenMore: 3, OpenPorts: 7}}
	rec := RecordOf(o, "a", "", "")
	if rec.Window == nil || rec.Window.Window != "last 24 hours" || rec.Window.OpenPorts != 7 || len(rec.Window.Open) != 0 || rec.Window.OpenMore != 0 {
		t.Errorf("window = %+v", rec.Window)
	}
	if len(o.Window.Open) != 1 {
		t.Error("the caller's digest must not be changed")
	}
}
