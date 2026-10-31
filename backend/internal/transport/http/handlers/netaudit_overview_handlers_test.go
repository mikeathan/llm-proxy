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

type fakeSeries struct {
	asked int
	days  []models.NetworkDay
	err   error
}

func (f *fakeSeries) Series(n int) ([]models.NetworkDay, error) { f.asked = n; return f.days, f.err }

type fakeBaselines struct {
	b    models.ScanBaseline
	read [2]string
	err  error
}

func (f *fakeBaselines) ReadBaseline(ws, a string) (models.ScanBaseline, error) {
	f.read = [2]string{ws, a}
	return f.b, f.err
}

func overviewHandlers(devices *fakeDevices, series *fakeSeries, b *fakeBaselines) *NetauditOverviewHandlers {
	return NewNetauditOverviewHandlers(devices, &fakePresence{}, series, b)
}

func getOverview(h *NetauditOverviewHandlers, query string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.Get(rec, httptest.NewRequest(http.MethodGet, "/admin/api/netaudit/overview"+query, nil))
	return rec
}

func TestOverviewHandler_JoinsTheInventoryTheLastAuditAndTheSeries(t *testing.T) {
	now := time.Now()
	devices := &fakeDevices{inv: models.DeviceInventory{Baselined: true, Devices: []models.NetworkDevice{{MAC: "aa:bb:cc:00:00:01", IP: "192.168.1.1", Name: "Router", Approved: true, FirstSeen: now.AddDate(0, -2, 0), LastSeen: now}}}}
	series := &fakeSeries{days: []models.NetworkDay{{Date: "2026-10-31", Devices: 1}}}
	bases := &fakeBaselines{b: models.ScanBaseline{ScannedAt: now, Findings: []models.ScanFinding{{Severity: netaudit.SeverityHigh}}}}
	rec := getOverview(overviewHandlers(devices, series, bases), "?workspace_id=ws&automation=nightly&days=30")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var got netaudit.Overview
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Posture != netaudit.PostureAttention || got.Findings.Serious != 1 || got.Devices.Known != 1 || len(got.Series) != 1 {
		t.Errorf("overview = %+v", got)
	}
	if bases.read != [2]string{"ws", "nightly"} || series.asked != 30 {
		t.Errorf("read %v, asked %d days", bases.read, series.asked)
	}
}

func TestOverviewHandler_WithoutAnAuditItStillShowsTheNetwork(t *testing.T) {
	bases := &fakeBaselines{err: errors.New("must not be read")}
	rec := getOverview(overviewHandlers(&fakeDevices{}, &fakeSeries{}, bases), "")
	if rec.Code != http.StatusOK || bases.read != [2]string{} {
		t.Errorf("status %d, baseline read %v", rec.Code, bases.read)
	}
}

func TestOverviewHandler_TheDaysAreBoundedAndMustBeANumber(t *testing.T) {
	series := &fakeSeries{}
	h := overviewHandlers(&fakeDevices{}, series, &fakeBaselines{})
	if rec := getOverview(h, "?days=100000"); rec.Code != http.StatusOK || series.asked != maxOverviewDays {
		t.Errorf("status %d, asked %d days", rec.Code, series.asked)
	}
	if rec := getOverview(h, "?days=abc"); rec.Code != http.StatusBadRequest {
		t.Errorf("a non-numeric days must be refused, got %d", rec.Code)
	}
}

func TestOverviewHandler_AReadFailureIsAServerError(t *testing.T) {
	if rec := getOverview(overviewHandlers(&fakeDevices{err: errors.New("disk")}, &fakeSeries{}, &fakeBaselines{}), ""); rec.Code != http.StatusInternalServerError {
		t.Errorf("status %d", rec.Code)
	}
}
