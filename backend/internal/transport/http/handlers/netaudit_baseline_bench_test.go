package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"llm-proxy/models"
)

// The baseline for plan P3b (network-stack-p3b-design.md section 4a): the two Network endpoints a stack move must not slow down, measured
// through the handler with the same fakes the tests use. Run them before and after each slice on the same machine:
//
//	go test ./internal/transport/http/handlers -run '^$' -bench 'Baseline' -benchmem -count 5
//
// They exercise the handler and its JSON encoding, which is the work a request does; the network and the sign-in check are not in them.

func BenchmarkBaseline_DeviceList(b *testing.B) {
	now := time.Now()
	devs := make([]models.NetworkDevice, 200)
	for i := range devs {
		devs[i] = models.NetworkDevice{MAC: fmt.Sprintf("00:1b:63:00:%02x:%02x", i/256, i%256), IP: fmt.Sprintf("192.168.1.%d", i%250), FirstSeen: now, LastSeen: now}
	}
	h := NewNetauditDevicesHandlers(&fakeDevices{inv: models.DeviceInventory{Baselined: true, Devices: devs}}, &fakePresence{})
	runHandler(b, h.List, "/admin/api/netaudit/devices")
}

func BenchmarkBaseline_Findings(b *testing.B) {
	findings := make([]models.ScanFinding, 500)
	for i := range findings {
		findings[i] = models.ScanFinding{Device: fmt.Sprintf("10.0.%d.%d", i/250, i%250), Port: 22 + i%40, Proto: "tcp", Check: "open_port", Severity: "info"}
	}
	baseline := models.ScanBaseline{ScannedAt: time.Now(), Engine: "nmap", Findings: findings, DevicesUp: 250}
	h := NewNetauditHandlers(nil, nil, fixedBaselines{b: baseline}, nil)
	runHandler(b, h.GetFindings, "/admin/api/netaudit/findings?workspace_id=ws&automation=nightly")
}

func runHandler(b *testing.B, h http.HandlerFunc, target string) {
	b.Helper()
	b.ReportAllocs()
	for b.Loop() {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code != http.StatusOK {
			b.Fatalf("status %d: %s", rec.Code, rec.Body.String())
		}
	}
}
