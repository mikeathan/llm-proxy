package persistence

import (
	"reflect"
	"testing"
	"time"

	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

func newBaselineManager(t *testing.T) *WorkspaceManager {
	t.Helper()
	tmp := t.TempDir()
	return NewWorkspaceManager(storage.NewPathResolver(tmp, tmp, tmp))
}

func sampleBaseline() models.ScanBaseline {
	return models.ScanBaseline{
		ScannedAt: time.Date(2026, 10, 25, 2, 0, 0, 0, time.UTC),
		Engine:    "nmap",
		Findings:  []models.ScanFinding{{Device: "192.168.1.5", Port: 22, Proto: "tcp", Check: "open_port", Severity: "info", Service: "ssh"}},
	}
}

func TestBaseline_NoEarlierScanIsAZeroBaseline(t *testing.T) {
	got, err := newBaselineManager(t).ReadBaseline("ws", "nightly")
	if err != nil || !got.ScannedAt.IsZero() || len(got.Findings) != 0 {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestBaseline_RoundTripsAndKeepsAutomationsApart(t *testing.T) {
	mgr := newBaselineManager(t)
	want := sampleBaseline()
	if err := mgr.WriteBaseline("ws", "nightly", want); err != nil {
		t.Fatal(err)
	}
	other := models.ScanBaseline{Engine: "portprobe"}
	if err := mgr.WriteBaseline("ws", "weekly", other); err != nil {
		t.Fatal(err)
	}

	got, err := mgr.ReadBaseline("ws", "nightly")
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("nightly = %+v, %v; want %+v (two automations must not share a baseline)", got, err, want)
	}
}

func TestBaseline_DeletingAnAutomationRemovesItsBaseline(t *testing.T) {
	mgr := newBaselineManager(t)
	for _, name := range []string{"nightly", "weekly"} {
		if err := mgr.WriteBaseline("ws", name, sampleBaseline()); err != nil {
			t.Fatal(err)
		}
	}
	if err := mgr.DeleteAutomationRuns("ws", "nightly"); err != nil {
		t.Fatal(err)
	}
	if got, _ := mgr.ReadBaseline("ws", "nightly"); len(got.Findings) != 0 {
		t.Errorf("a deleted automation kept its baseline: %+v", got)
	}
	if got, _ := mgr.ReadBaseline("ws", "weekly"); len(got.Findings) != 1 {
		t.Errorf("another automation's baseline must survive: %+v", got)
	}
}
