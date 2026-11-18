package radio

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"llm-proxy-stacks/sdk"
)

func get(t *testing.T, r *listenRig, path string, v any) {
	t.Helper()
	resp, err := r.s.Handle(context.Background(), stacksdk.Request{Method: http.MethodGet, Path: path})
	if err != nil || resp.Status != http.StatusOK {
		t.Fatalf("%s: %+v %v", path, resp, err)
	}
	if err := json.Unmarshal(resp.Body, v); err != nil {
		t.Fatal(err)
	}
}

func TestAirShowsCountsByClassAndTheRulesAndNothingAboutAnyDevice(t *testing.T) {
	r := devRig(t)
	rec, store, err := r.s.recorder(r.s.h)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 14; i++ {
		cycleOf(t, rec, r.now, data(neighbour), data(neighbour))
	}
	var v AirView
	get(t, r, AirPath, &v)
	if v.Rules.Version != 3 || !v.Rules.Provisional || v.Listened != 14 || v.Total != 1 || v.Counts["infrastructure"] != 1 || v.Counts["personal"] != 0 {
		t.Errorf("%+v", v)
	}
	if len(v.Changes) == 0 || v.Changes[0].Reason == "" {
		t.Errorf("%+v", v.Changes)
	}
	resp, _ := r.s.Handle(context.Background(), stacksdk.Request{Method: http.MethodGet, Path: AirPath})
	tag, _ := fakeVault{}.Tag(context.Background(), tagDevice, neighbour[:])
	if strings.Contains(string(resp.Body), hex.EncodeToString(tag)) || strings.Contains(string(resp.Body), "b8:27") {
		t.Error("the page says counts only: no tag and no address of another person")
	}
	_ = store
}

func TestTheNetworksViewShowsTheCoresListAndCannotChangeIt(t *testing.T) {
	r := devRig(t)
	r.nets.set([]string{"home"})
	if resp, _ := r.s.Handle(context.Background(), stacksdk.Request{Method: http.MethodPut, Path: NetworksPath, Body: []byte(`{"names":["x"]}`)}); resp.Status == http.StatusOK {
		t.Errorf("the names are set on the Network page only: PUT answered %d", resp.Status)
	}
	rec, _, _ := r.s.recorder(r.s.h)
	if err := rec.SyncOwnNetworks(context.Background()); err != nil {
		t.Fatal(err)
	}
	record(t, rec, beacon("home"), t0)
	var v NetworksView
	get(t, r, NetworksPath, &v)
	if len(v.Names) != 1 || v.Names[0] != "home" || len(v.AccessPoints) != 1 || v.AccessPoints[0].Confirmed || v.AccessPoints[0].Name != "home" {
		t.Fatalf("a beacon with the owner's name proposes an access point: %+v", v)
	}
	r.nets.set([]string{"home"}, macAP) // confirmed on the Network page
	get(t, r, NetworksPath, &v)
	if !v.AccessPoints[0].Confirmed || v.AccessPoints[0].ConfirmedBefore {
		t.Errorf("the view reads the core's list each time: %+v", v)
	}
}
