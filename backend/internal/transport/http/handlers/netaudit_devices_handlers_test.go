package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/models"
)

type fakeDevices struct {
	inv       models.DeviceInventory
	named     [2]string
	approved  string
	forgotten string
	confirmed bool
	err       error
}

func (f *fakeDevices) View() (models.DeviceInventory, error) { return f.inv, f.err }
func (f *fakeDevices) Name(mac, name string) error           { f.named = [2]string{mac, name}; return f.err }
func (f *fakeDevices) Approve(mac string) error              { f.approved = mac; return f.err }
func (f *fakeDevices) Forget(mac string) error               { f.forgotten = mac; return f.err }
func (f *fakeDevices) ConfirmGateway() error                 { f.confirmed = true; return f.err }

type fakePresence struct {
	enabled bool
	minutes int
	err     error
}

func (f *fakePresence) View() netaudit.PresenceView {
	return netaudit.PresenceView{Enabled: f.enabled, IntervalMinutes: 10, AllowedIntervals: netaudit.AllowedPresenceIntervals}
}
func (f *fakePresence) Set(enabled bool, minutes int) error {
	f.enabled, f.minutes = enabled, minutes
	return f.err
}
func (f *fakePresence) Status() netaudit.PresenceStatus {
	return netaudit.PresenceStatus{Problem: "this computer cannot list the devices on its network (only Linux can)"}
}

func withMAC(h http.HandlerFunc, method, body, mac string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/admin/api/netaudit/devices/"+mac, strings.NewReader(body))
	req.SetPathValue("mac", mac)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestDevicesHandlers_ListShowsTheInventoryView(t *testing.T) {
	now := time.Now()
	devices := &fakeDevices{inv: models.DeviceInventory{Baselined: true, Devices: []models.NetworkDevice{
		{MAC: "aa:bb:cc:00:00:01", IP: "192.168.1.5", FirstSeen: now, LastSeen: now},
	}}}
	h := NewNetauditDevicesHandlers(devices, &fakePresence{})
	rec := do(h.List, http.MethodGet, "/admin/api/netaudit/devices", "")
	var got netaudit.InventoryView
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil || len(got.Devices) != 1 || !got.Devices[0].New {
		t.Fatalf("list = %d %s", rec.Code, rec.Body.String())
	}
}

func TestDevicesHandlers_NamingAndApproving(t *testing.T) {
	devices := &fakeDevices{}
	h := NewNetauditDevicesHandlers(devices, &fakePresence{})
	if rec := withMAC(h.Update, http.MethodPut, `{"name":"Printer"}`, "aa:bb:cc:00:00:01"); rec.Code != http.StatusOK || devices.named != [2]string{"aa:bb:cc:00:00:01", "Printer"} {
		t.Errorf("name = %d %s %v", rec.Code, rec.Body.String(), devices.named)
	}
	if rec := withMAC(h.Update, http.MethodPut, `{"approved":true}`, "aa:bb:cc:00:00:02"); rec.Code != http.StatusOK || devices.approved != "aa:bb:cc:00:00:02" {
		t.Errorf("approve = %d %s", rec.Code, rec.Body.String())
	}
	if rec := withMAC(h.Update, http.MethodPut, `{}`, "aa:bb:cc:00:00:02"); rec.Code != http.StatusBadRequest {
		t.Errorf("an empty change is a mistake: %d", rec.Code)
	}
}

func TestDevicesHandlers_RefusalsHaveTheRightStatus(t *testing.T) {
	cases := map[error]int{
		netaudit.ErrBadMAC: http.StatusBadRequest, netaudit.ErrBadDeviceName: http.StatusBadRequest,
		netaudit.ErrNoSuchDevice: http.StatusNotFound, errors.New("disk full"): http.StatusInternalServerError,
	}
	for err, want := range cases {
		h := NewNetauditDevicesHandlers(&fakeDevices{err: err}, &fakePresence{})
		if rec := withMAC(h.Update, http.MethodPut, `{"name":"x"}`, "aa:bb:cc:00:00:01"); rec.Code != want {
			t.Errorf("%v -> %d, want %d", err, rec.Code, want)
		}
	}
}

func TestDevicesHandlers_ForgetAndConfirmGateway(t *testing.T) {
	devices := &fakeDevices{}
	h := NewNetauditDevicesHandlers(devices, &fakePresence{})
	if rec := withMAC(h.Forget, http.MethodDelete, "", "aa:bb:cc:00:00:01"); rec.Code != http.StatusOK || devices.forgotten != "aa:bb:cc:00:00:01" {
		t.Errorf("forget = %d", rec.Code)
	}
	if rec := do(h.ConfirmGateway, http.MethodPost, "/admin/api/netaudit/devices/gateway/confirm", ""); rec.Code != http.StatusOK || !devices.confirmed {
		t.Errorf("confirm = %d", rec.Code)
	}
}

func TestPresenceHandlers_GetAndSet(t *testing.T) {
	presence := &fakePresence{}
	h := NewNetauditDevicesHandlers(&fakeDevices{}, presence)
	rec := do(h.GetPresence, http.MethodGet, "/admin/api/netaudit/presence", "")
	var got struct {
		Enabled bool   `json:"enabled"`
		Problem string `json:"problem"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil || got.Problem == "" {
		t.Fatalf("get = %d %s (the problem must be shown, not hidden)", rec.Code, rec.Body.String())
	}
	if rec := do(h.PutPresence, http.MethodPut, "/admin/api/netaudit/presence", `{"enabled":true,"interval_minutes":15}`); rec.Code != http.StatusOK || !presence.enabled || presence.minutes != 15 {
		t.Errorf("put = %d %s", rec.Code, rec.Body.String())
	}
	presence.err = netaudit.ErrBadPresenceInterval
	if rec := do(h.PutPresence, http.MethodPut, "/admin/api/netaudit/presence", `{"enabled":true,"interval_minutes":3}`); rec.Code != http.StatusBadRequest {
		t.Errorf("a bad interval is a 400: %d", rec.Code)
	}
}
