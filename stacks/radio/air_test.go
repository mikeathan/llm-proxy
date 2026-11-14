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
	if v.Rules.Version != 1 || !v.Rules.Provisional || v.Listened != 14 || v.Total != 1 || v.Counts["infrastructure"] != 1 || v.Counts["personal"] != 0 {
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

func TestOwnNetworkNamesProposeAccessPointsAndTheOwnerConfirmsThem(t *testing.T) {
	r := devRig(t)
	resp, err := r.s.Handle(context.Background(), stacksdk.Request{Method: http.MethodPut, Path: NetworksPath, Body: []byte(`{"names":["home"," "]}`)})
	if err != nil || resp.Status != http.StatusOK {
		t.Fatalf("%+v %v", resp, err)
	}
	if resp, _ := r.s.Handle(context.Background(), stacksdk.Request{Method: http.MethodPut, Path: NetworksPath, Body: []byte(`{"names":["` + strings.Repeat("x", 40) + `"]}`)}); resp.Status != http.StatusBadRequest {
		t.Errorf("an SSID is at most 32 bytes: %d", resp.Status)
	}
	rec, _, _ := r.s.recorder(r.s.h)
	record(t, rec, beacon("home"), t0)
	var v NetworksView
	get(t, r, NetworksPath, &v)
	if len(v.AccessPoints) != 1 || v.AccessPoints[0].Confirmed || v.AccessPoints[0].Name != "home" {
		t.Fatalf("a beacon with the owner's name proposes an access point: %+v", v)
	}
	id := v.AccessPoints[0].ID
	resp, _ = r.s.Handle(context.Background(), stacksdk.Request{Method: http.MethodPost, Path: NetworkConfirmPath, Params: map[string]string{"id": id}})
	if resp.Status != http.StatusOK {
		t.Fatalf("%+v", resp)
	}
	get(t, r, NetworksPath, &v)
	if !v.AccessPoints[0].Confirmed {
		t.Errorf("%+v", v)
	}
	resp, _ = r.s.Handle(context.Background(), stacksdk.Request{Method: http.MethodDelete, Path: NetworkPath, Params: map[string]string{"id": id}})
	if resp.Status != http.StatusOK {
		t.Fatalf("%+v", resp)
	}
	if resp, _ = r.s.Handle(context.Background(), stacksdk.Request{Method: http.MethodPost, Path: NetworkConfirmPath, Params: map[string]string{"id": "zz"}}); resp.Status != http.StatusNotFound {
		t.Errorf("%d", resp.Status)
	}
}
