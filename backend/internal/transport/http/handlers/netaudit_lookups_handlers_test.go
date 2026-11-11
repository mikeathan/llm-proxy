package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"llm-proxy/models"
)

type fakeLookups struct{ on bool }

func (f fakeLookups) DeviceLookups(time.Time) (bool, time.Time, []models.DeviceLookups) {
	return f.on, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), []models.DeviceLookups{{MAC: "aa", Flags: []string{}, Names: []models.DeviceLookupName{{Name: "a.example.com", Domain: "example.com"}}}}
}

func TestTheLookupsAreReadPerDeviceWithWhetherTheWatchIsOn(t *testing.T) {
	w := httptest.NewRecorder()
	NewNetauditLookupsHandlers(func() DeviceLookupsSource { return fakeLookups{on: true} }).Get(w, httptest.NewRequest(http.MethodGet, "/", nil))
	body := w.Body.String()
	if w.Code != http.StatusOK || !strings.Contains(body, `"enabled":true`) || !strings.Contains(body, `"mac":"aa"`) || !strings.Contains(body, `"since":"2026-11-01`) {
		t.Fatalf("%d %s", w.Code, body)
	}
}

func TestLookupsBeforeTheServicesAreWiredAreAnEmptyListNotAnError(t *testing.T) {
	w := httptest.NewRecorder()
	NewNetauditLookupsHandlers(func() DeviceLookupsSource { return nil }).Get(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"devices":[]`) || !strings.Contains(w.Body.String(), `"enabled":false`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}
