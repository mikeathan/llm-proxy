package automation

import (
	"errors"
	"testing"

	"llm-proxy/models"
)

func registerAudit(t *testing.T, d *Dispatcher, name string) {
	t.Helper()
	if err := d.Register("ws", &models.Automation{
		Name: name, Trigger: models.TriggerConfig{Type: "manual"}, TaskFile: "task.md",
		Profile: models.AuditorProfileID, Scan: &models.ScanSettings{Targets: []string{"192.168.1.0/24"}, Depth: models.ScanDepthQuick},
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.persistence.WriteTaskFile("ws", "task.md", "audit the network"); err != nil {
		t.Fatal(err)
	}
}

func TestTrigger_ScanTargetRunsTheAuditOnOneDeviceWithItsOwnSettings(t *testing.T) {
	exec := newRecordingExecutor("done", nil)
	d := newLaneDispatcher(t, exec)
	registerAudit(t, d, "audit")

	if _, err := d.Trigger("ws", "audit", RunOptions{ScanTarget: "192.168.1.20"}); err != nil {
		t.Fatalf("trigger: %v", err)
	}
	waitFor(t, "the run", func() bool { return exec.count() == 1 })

	exec.mu.Lock()
	got := exec.requests[0].Scan
	exec.mu.Unlock()
	if got == nil || len(got.Targets) != 1 || got.Targets[0] != "192.168.1.20" || got.Depth != models.ScanDepthQuick {
		t.Errorf("the run scans only the device, with the audit's own depth: %+v", got)
	}
	if entry, _ := d.registry.Get("ws", "audit"); len(entry.Scan.Targets) != 1 || entry.Scan.Targets[0] != "192.168.1.0/24" {
		t.Errorf("the audit's saved targets must not change: %+v", entry.Scan)
	}
}

func TestTrigger_ScanTargetIsRefusedWhenItIsNotOneAddressOrTheAutomationIsNotAnAudit(t *testing.T) {
	d := newLaneDispatcher(t, newRecordingExecutor("done", nil))
	registerAudit(t, d, "audit")
	registerManualAutomation(t, d, "ws", "plain")

	for _, target := range []string{"192.168.1.0/24", "printer.local", "not an address", "--script=x"} {
		if _, err := d.Trigger("ws", "audit", RunOptions{ScanTarget: target}); !errors.Is(err, ErrBadScanTarget) {
			t.Errorf("target %q: err = %v, want ErrBadScanTarget", target, err)
		}
	}
	if _, err := d.Trigger("ws", "plain", RunOptions{ScanTarget: "192.168.1.20"}); !errors.Is(err, ErrNotAnAudit) {
		t.Errorf("an automation that is not an audit: err = %v, want ErrNotAnAudit", err)
	}
}
