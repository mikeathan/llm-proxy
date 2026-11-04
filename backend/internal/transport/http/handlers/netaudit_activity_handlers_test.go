package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/models"
)

func getActivity(h *NetauditActivityHandlers, query string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.Get(rec, httptest.NewRequest(http.MethodGet, "/admin/api/netaudit/activity"+query, nil))
	return rec
}

func TestActivityHandler_JoinsTheInventoryTheAlertsTheSeriesAndTheLastAudit(t *testing.T) {
	now := time.Now()
	devices := &fakeDevices{inv: models.DeviceInventory{Baselined: true, Devices: []models.NetworkDevice{{MAC: "aa:bb:cc:00:00:09", IP: "192.168.1.9", FirstSeen: now.Add(-2 * time.Hour), LastSeen: now}}}}
	alerts := fakeAlerts{items: []models.NetworkAlert{{ID: 1, Kind: models.NetworkAlertPresence, Message: "m", At: now.Add(-time.Hour), Count: 1}}}
	series := &fakeSeries{days: []models.NetworkDay{{Date: now.Format("2006-01-02"), PortsOpened: 1}}}
	bases := &fakeBaselines{b: models.ScanBaseline{ScannedAt: now.Add(-3 * time.Hour), NewKeys: []string{"k"}}}
	h := NewNetauditActivityHandlers(devices, alerts, series, bases)
	rec := getActivity(h, "?workspace_id=ws&automation=nightly&limit=50")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var got netaudit.Activity
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, r := range got.Rows {
		kinds[r.Kind] = true
	}
	for _, want := range []string{netaudit.ActivityNewDevice, netaudit.ActivityAlert, netaudit.ActivityPortsChanged, netaudit.ActivityAudit} {
		if !kinds[want] {
			t.Errorf("missing a %q row: %+v", want, got.Rows)
		}
	}
	if bases.read != [2]string{"ws", "nightly"} {
		t.Errorf("the audit read was %v", bases.read)
	}
}

func TestActivityHandler_WithoutAnAuditItHasNoAuditRowAndAQuietNetworkEncodesAsAnEmptyList(t *testing.T) {
	h := NewNetauditActivityHandlers(&fakeDevices{}, fakeAlerts{}, &fakeSeries{}, &fakeBaselines{})
	rec := getActivity(h, "")
	if rec.Code != http.StatusOK || rec.Body.String() != "{\"rows\":[]}\n" && rec.Body.String() != "{\"rows\":[]}" {
		t.Errorf("status %d body %q", rec.Code, rec.Body)
	}
}

func TestActivityHandler_RefusesABadLimitAndReportsAFailedReadWithoutHidingIt(t *testing.T) {
	ok := NewNetauditActivityHandlers(&fakeDevices{}, fakeAlerts{}, &fakeSeries{}, &fakeBaselines{})
	for _, q := range []string{"?limit=abc", "?limit=0", "?limit=-3"} {
		if rec := getActivity(ok, q); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d", q, rec.Code)
		}
	}
	broken := map[string]*NetauditActivityHandlers{
		"devices":   NewNetauditActivityHandlers(&fakeDevices{err: errors.New("boom")}, fakeAlerts{}, &fakeSeries{}, &fakeBaselines{}),
		"series":    NewNetauditActivityHandlers(&fakeDevices{}, fakeAlerts{}, &fakeSeries{err: errors.New("boom")}, &fakeBaselines{}),
		"baselines": NewNetauditActivityHandlers(&fakeDevices{}, fakeAlerts{}, &fakeSeries{}, &fakeBaselines{err: errors.New("boom")}),
	}
	for name, h := range broken {
		if rec := getActivity(h, "?workspace_id=ws&automation=a"); rec.Code != http.StatusInternalServerError {
			t.Errorf("%s: status %d, want 500 and not a quiet list", name, rec.Code)
		}
	}
}
