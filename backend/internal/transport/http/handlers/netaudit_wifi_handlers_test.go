package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"llm-proxy/internal/core/netaudit"
)

type fakeWifi struct {
	view      netaudit.WifiView
	looked    int
	lookErr   error
	confirmed [][]string
	confirmEr error
	added     []string
	removed   []string
	nameErr   error
}

func (f *fakeWifi) View() netaudit.WifiView    { return f.view }
func (f *fakeWifi) Look(context.Context) error { f.looked++; return f.lookErr }
func (f *fakeWifi) Confirm(_ string, a []string) error {
	f.confirmed = append(f.confirmed, a)
	return f.confirmEr
}
func (f *fakeWifi) AddNetwork(n string) error    { f.added = append(f.added, n); return f.nameErr }
func (f *fakeWifi) RemoveNetwork(n string) error { f.removed = append(f.removed, n); return f.nameErr }

func wifiHandlers(f *fakeWifi, ready bool) *NetauditWifiHandlers {
	why := ""
	if !ready {
		why = "nmcli is not installed"
	}
	return NewNetauditWifiHandlers(f, f, func() (bool, string) { return ready, why })
}

func TestWifiHandlers_GetShowsTheCardsStateAndWhyTheLookCannotRun(t *testing.T) {
	f := &fakeWifi{view: netaudit.WifiView{Enabled: true, Networks: []netaudit.WifiNetworkView{{Name: "home", Confirmed: []string{"aa:bb:cc:00:00:01"}, Seen: []netaudit.WifiSeen{}}}}}
	rec := do(wifiHandlers(f, false).Get, http.MethodGet, "/admin/api/netaudit/wifi", "")
	for _, want := range []string{`"enabled":true`, `"name":"home"`, `"ready":false`, `"why":"nmcli is not installed"`} {
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("missing %q in %d %s", want, rec.Code, rec.Body.String())
		}
	}
}

func TestWifiHandlers_LookRunsOnlyWhenTheLookCanRunAndSaysWhenItFailed(t *testing.T) {
	f := &fakeWifi{}
	if rec := do(wifiHandlers(f, false).Look, http.MethodPost, "/admin/api/netaudit/wifi/look", ""); rec.Code != http.StatusConflict || f.looked != 0 {
		t.Errorf("not ready: %d, looked %d times", rec.Code, f.looked)
	}
	if rec := do(wifiHandlers(f, true).Look, http.MethodPost, "/admin/api/netaudit/wifi/look", ""); rec.Code != http.StatusOK || f.looked != 1 {
		t.Errorf("ready: %d, looked %d times", rec.Code, f.looked)
	}
	f.lookErr = errors.New("NetworkManager is not running")
	if rec := do(wifiHandlers(f, true).Look, http.MethodPost, "/admin/api/netaudit/wifi/look", ""); rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "NetworkManager is not running") {
		t.Errorf("failed look: %d %s", rec.Code, rec.Body.String())
	}
}

func TestWifiHandlers_NamesAndConfirmationReachTheServiceAndRefusalsAreSaid(t *testing.T) {
	f := &fakeWifi{}
	h := wifiHandlers(f, true)
	if rec := do(h.AddNetwork, http.MethodPost, "/admin/api/netaudit/wifi/networks", `{"name":"home"}`); rec.Code != http.StatusOK || len(f.added) != 1 || f.added[0] != "home" {
		t.Errorf("add = %d %v", rec.Code, f.added)
	}
	if rec := do(h.RemoveNetwork, http.MethodDelete, "/admin/api/netaudit/wifi/networks?name=home", ""); rec.Code != http.StatusOK || len(f.removed) != 1 || f.removed[0] != "home" {
		t.Errorf("remove = %d %v", rec.Code, f.removed)
	}
	if rec := do(h.Confirm, http.MethodPost, "/admin/api/netaudit/wifi/confirm", `{"name":"home","addresses":["aa:bb:cc:00:00:01"]}`); rec.Code != http.StatusOK || len(f.confirmed) != 1 {
		t.Errorf("confirm = %d %v", rec.Code, f.confirmed)
	}
	f.confirmEr, f.nameErr = errors.New("aa:bb:cc:00:00:09 was not seen advertising \"home\" in the last look; look again first"), errors.New("a Wi-Fi network name is 1 to 32 characters")
	if rec := do(h.Confirm, http.MethodPost, "/x", `{"name":"home","addresses":["aa:bb:cc:00:00:09"]}`); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "look again first") {
		t.Errorf("a refused confirmation = %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(h.AddNetwork, http.MethodPost, "/x", `{"name":""}`); rec.Code != http.StatusBadRequest {
		t.Errorf("a refused name = %d", rec.Code)
	}
	if rec := do(h.AddNetwork, http.MethodPost, "/x", `nope`); rec.Code != http.StatusBadRequest {
		t.Errorf("a malformed body = %d", rec.Code)
	}
}
