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
	"llm-proxy/internal/core/netaudit/names"
	"llm-proxy/models"
)

type fakeDevices struct {
	inv       models.DeviceInventory
	named     [2]string
	approved  string
	forgotten string
	confirmed bool
	important map[string]bool
	cleared   [2]string
	err       error
}

func (f *fakeDevices) View() (models.DeviceInventory, error) { return f.inv, f.err }
func (f *fakeDevices) Name(mac, name string) error           { f.named = [2]string{mac, name}; return f.err }
func (f *fakeDevices) Approve(mac string) error              { f.approved = mac; return f.err }
func (f *fakeDevices) Forget(mac string) error               { f.forgotten = mac; return f.err }
func (f *fakeDevices) ConfirmGateway() error                 { f.confirmed = true; return f.err }
func (f *fakeDevices) SetImportant(mac string, on bool) error {
	f.important = map[string]bool{mac: on}
	return f.err
}

func (f *fakeDevices) ClearProbe(mac, kind string) error {
	f.cleared = [2]string{mac, kind}
	return f.err
}

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

// The maker of a card comes from the lookup the composition root gives the handler (the registry is the Network stack's).
func TestDevicesHandlers_ListNamesTheMakerThroughTheLookup(t *testing.T) {
	now := time.Now()
	devices := &fakeDevices{inv: models.DeviceInventory{Devices: []models.NetworkDevice{
		{MAC: "aa:bb:cc:00:00:01", IP: "192.168.1.5", FirstSeen: now, LastSeen: now},
	}}}
	maker := netaudit.VendorLookup(func(string) string { return "Acme Networks" })
	vendorOf := func(h *NetauditDevicesHandlers) string {
		var got netaudit.InventoryView
		rec := do(h.List, http.MethodGet, "/admin/api/netaudit/devices", "")
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil || len(got.Devices) != 1 {
			t.Fatalf("list = %d %s", rec.Code, rec.Body.String())
		}
		return got.Devices[0].Vendor
	}
	if v := vendorOf(NewNetauditDevicesHandlers(devices, &fakePresence{}).WithVendors(maker)); v != "Acme Networks" {
		t.Errorf("vendor = %q", v)
	}
	if v := vendorOf(NewNetauditDevicesHandlers(devices, &fakePresence{})); v != "" {
		t.Errorf("with no lookup there is no maker, got %q", v)
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

type fakeSuggestions map[string]string

func (f fakeSuggestions) All() map[string]string { return f }
func (f fakeSuggestions) Dismiss(mac string) error {
	if _, ok := f[mac]; !ok {
		return names.ErrNoSuggestion
	}
	delete(f, mac)
	return nil
}

type fakePorts map[string]models.DevicePortRecord

func (f fakePorts) All() map[string]models.DevicePortRecord { return f }

func TestDevicesHandlers_ListOffersASuggestedNameOnlyForADeviceTheOperatorHasNotNamed(t *testing.T) {
	now := time.Now()
	devices := &fakeDevices{inv: models.DeviceInventory{Baselined: true, Devices: []models.NetworkDevice{
		{MAC: "aa:bb:cc:00:00:01", IP: "192.168.1.5", FirstSeen: now, LastSeen: now},
		{MAC: "aa:bb:cc:00:00:02", IP: "192.168.1.6", Name: "Printer", FirstSeen: now, LastSeen: now},
	}}}
	h := NewNetauditDevicesHandlers(devices, &fakePresence{}).WithSuggestions(fakeSuggestions{"aa:bb:cc:00:00:01": "Samsung", "aa:bb:cc:00:00:02": "Brother"})
	rec := do(h.List, http.MethodGet, "/admin/api/netaudit/devices", "")
	var got netaudit.InventoryView
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	by := map[string]netaudit.DeviceView{}
	for _, d := range got.Devices {
		by[d.MAC] = d
	}
	if by["aa:bb:cc:00:00:01"].SuggestedName != "Samsung" || by["aa:bb:cc:00:00:02"].SuggestedName != "" {
		t.Errorf("suggestions = %q, %q", by["aa:bb:cc:00:00:01"].SuggestedName, by["aa:bb:cc:00:00:02"].SuggestedName)
	}
}

func TestDevicesHandlers_ListShowsTheOpenPortsFromTheLastScanBesideEachDevice(t *testing.T) {
	now := time.Now()
	devices := &fakeDevices{inv: models.DeviceInventory{Baselined: true, Devices: []models.NetworkDevice{
		{MAC: "aa:bb:cc:00:00:01", IP: "192.168.1.5", FirstSeen: now, LastSeen: now},
		{MAC: "aa:bb:cc:00:00:02", IP: "192.168.1.6", FirstSeen: now, LastSeen: now},
	}}}
	scanned := now.Add(-time.Hour)
	h := NewNetauditDevicesHandlers(devices, &fakePresence{}).WithPorts(fakePorts{
		"aa:bb:cc:00:00:01": {MAC: "aa:bb:cc:00:00:01", ScannedAt: scanned, Ports: []models.PortRef{{Port: 22, Proto: "tcp"}, {Port: 80, Proto: "tcp"}}},
	})
	var got netaudit.InventoryView
	if err := json.Unmarshal(do(h.List, http.MethodGet, "/admin/api/netaudit/devices", "").Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	by := map[string]netaudit.DeviceView{}
	for _, d := range got.Devices {
		by[d.MAC] = d
	}
	if d := by["aa:bb:cc:00:00:01"]; len(d.OpenPorts) != 2 || d.OpenPorts[0] != 22 || d.PortsScannedAt.IsZero() {
		t.Errorf("a scanned device lists its ports: %+v", d)
	}
	if d := by["aa:bb:cc:00:00:02"]; d.OpenPorts != nil || !d.PortsScannedAt.IsZero() {
		t.Errorf("a device never scanned has no ports to show: %+v", d)
	}
}

type fakeSilence map[string]bool

func (f fakeSilence) SilentDevices(models.DeviceInventory) map[string]bool { return f }

func TestDevicesHandlers_ListMarksTheDevicesThatAskTheRouterForNothing(t *testing.T) {
	now := time.Now()
	devices := &fakeDevices{inv: models.DeviceInventory{Baselined: true, Devices: []models.NetworkDevice{
		{MAC: "aa:bb:cc:00:00:01", IP: "192.168.1.5", FirstSeen: now, LastSeen: now},
		{MAC: "aa:bb:cc:00:00:02", IP: "192.168.1.6", FirstSeen: now, LastSeen: now},
	}}}
	h := NewNetauditDevicesHandlers(devices, &fakePresence{}).WithDNSSilence(fakeSilence{"aa:bb:cc:00:00:01": true})
	var got netaudit.InventoryView
	if err := json.Unmarshal(do(h.List, http.MethodGet, "/admin/api/netaudit/devices", "").Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	by := map[string]netaudit.DeviceView{}
	for _, d := range got.Devices {
		by[d.MAC] = d
	}
	if !by["aa:bb:cc:00:00:01"].DNSSilent || by["aa:bb:cc:00:00:02"].DNSSilent {
		t.Errorf("silent flags = %v, %v; only the first device is silent", by["aa:bb:cc:00:00:01"].DNSSilent, by["aa:bb:cc:00:00:02"].DNSSilent)
	}
	plain := NewNetauditDevicesHandlers(devices, &fakePresence{})
	if strings.Contains(do(plain.List, http.MethodGet, "/admin/api/netaudit/devices", "").Body.String(), "dns_silent") {
		t.Error("without a watch no device carries the flag")
	}
}

func TestDevicesHandlers_DismissSuggestion(t *testing.T) {
	now := time.Now()
	devices := &fakeDevices{inv: models.DeviceInventory{Baselined: true, Devices: []models.NetworkDevice{{MAC: "aa:bb:cc:00:00:01", IP: "192.168.1.5", FirstSeen: now, LastSeen: now}}}}
	sugg := fakeSuggestions{"aa:bb:cc:00:00:01": "Samsung"}
	h := NewNetauditDevicesHandlers(devices, &fakePresence{}).WithSuggestions(sugg)
	post := func(mac string) int {
		req := httptest.NewRequest(http.MethodPost, "/admin/api/netaudit/devices/"+mac+"/dismiss-suggestion", nil)
		req.SetPathValue("mac", mac)
		rec := httptest.NewRecorder()
		h.DismissSuggestion(rec, req)
		return rec.Code
	}
	if code := post("aa:bb:cc:00:00:01"); code != http.StatusOK || len(sugg) != 0 {
		t.Errorf("dismiss = %d, suggestions left = %v", code, sugg)
	}
	if code := post("aa:bb:cc:00:00:01"); code != http.StatusNotFound {
		t.Errorf("nothing left to dismiss is a 404: %d", code)
	}
}

func TestDevicesHandlers_MarkingADeviceImportantApprovesNothing(t *testing.T) {
	devices := &fakeDevices{}
	h := NewNetauditDevicesHandlers(devices, &fakePresence{})
	if rec := withMAC(h.Update, http.MethodPut, `{"important":true}`, "aa:bb:cc:00:00:01"); rec.Code != http.StatusOK || !devices.important["aa:bb:cc:00:00:01"] || devices.approved != "" {
		t.Errorf("mark = %d %s %v approved=%q", rec.Code, rec.Body.String(), devices.important, devices.approved)
	}
	if rec := withMAC(h.Update, http.MethodPut, `{"important":false}`, "aa:bb:cc:00:00:01"); rec.Code != http.StatusOK || devices.important["aa:bb:cc:00:00:01"] {
		t.Errorf("unmark = %d %v", rec.Code, devices.important)
	}
}

func TestNetauditDevices_UpdateClearsOneKeptProbeResult(t *testing.T) {
	devices := &fakeDevices{}
	h := NewNetauditDevicesHandlers(devices, &fakePresence{})
	if rec := withMAC(h.Update, http.MethodPut, `{"clear_probe":"udp"}`, "aa:bb:cc:00:00:01"); rec.Code != http.StatusOK || devices.cleared != [2]string{"aa:bb:cc:00:00:01", "udp"} {
		t.Errorf("clear = %d %s %v", rec.Code, rec.Body.String(), devices.cleared)
	}
	devices.err = netaudit.ErrBadProbeKind
	if rec := withMAC(h.Update, http.MethodPut, `{"clear_probe":"x"}`, "aa:bb:cc:00:00:01"); rec.Code != http.StatusBadRequest {
		t.Errorf("bad kind = %d", rec.Code)
	}
}
