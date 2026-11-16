package radio

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"llm-proxy-stacks/sdk"
)

func devRig(t *testing.T) *listenRig {
	t.Helper()
	return newRig(t, `{"enabled":true}`, okWindow)
}

func call(t *testing.T, r *listenRig, method, path, body string) (stacksdk.Response, error) {
	t.Helper()
	req := stacksdk.Request{Method: method, Path: path, Body: []byte(body)}
	if i := strings.LastIndex(path, "/"); method == http.MethodDelete && strings.HasPrefix(path, DevicesPath+"/") {
		req.Params = map[string]string{"id": path[i+1:]} // the path stays as requested, as the core sends it
	}
	return r.s.Handle(context.Background(), req)
}

func TestOwnerDevicesAreAddedListedAndDeletedWithoutShowingTheAddress(t *testing.T) {
	r := devRig(t)
	resp, err := call(t, r, http.MethodPost, DevicesPath, `{"address":"3A:91:5C:D2:7E:44","label":"my phone"}`)
	if err != nil || resp.Status != http.StatusCreated {
		t.Fatalf("%+v %v", resp, err)
	}
	resp, _ = call(t, r, http.MethodGet, DevicesPath, "")
	var list struct {
		Devices []OwnerDevice `json:"devices"`
	}
	if err := json.Unmarshal(resp.Body, &list); err != nil || len(list.Devices) != 1 || list.Devices[0].Label != "my phone" {
		t.Fatalf("%s %v", resp.Body, err)
	}
	if bytes.Contains(bytes.ToLower(resp.Body), []byte("3a:91")) {
		t.Error("the list shows the address")
	}
	// The same address again renames, it does not add a second row.
	_, _ = call(t, r, http.MethodPost, DevicesPath, `{"address":"3a-91-5c-d2-7e-44","label":"phone"}`)
	resp, _ = call(t, r, http.MethodGet, DevicesPath, "")
	_ = json.Unmarshal(resp.Body, &list)
	if len(list.Devices) != 1 || list.Devices[0].Label != "phone" {
		t.Errorf("%s", resp.Body)
	}
	if resp, _ = call(t, r, http.MethodDelete, DevicesPath+"/"+list.Devices[0].ID, ""); resp.Status != http.StatusOK {
		t.Fatalf("%d", resp.Status)
	}
	resp, _ = call(t, r, http.MethodGet, DevicesPath, "")
	_ = json.Unmarshal(resp.Body, &list)
	if len(list.Devices) != 0 {
		t.Errorf("%s", resp.Body)
	}
}

func TestOwnerDeviceRefusesWhatIsNotAnAddressOrIsAGroupAddress(t *testing.T) {
	r := devRig(t)
	for _, bad := range []string{`{`, `{"address":"nope","label":"x"}`, `{"address":"ff:ff:ff:ff:ff:ff","label":"x"}`, `{"address":"01:00:5e:00:00:01","label":"x"}`, `{"address":"3a:91:5c:d2:7e:44","label":""}`, `{"address":"3a:91:5c:d2:7e:44","label":"` + strings.Repeat("x", 61) + `"}`, `{"address":"3a:91:5c:d2:7e:44","label":"x","extra":1}`} {
		if resp, _ := call(t, r, http.MethodPost, DevicesPath, bad); resp.Status != http.StatusBadRequest {
			t.Errorf("%s: %d", bad, resp.Status)
		}
	}
	if resp, _ := call(t, r, http.MethodDelete, DevicesPath+"/zz", ""); resp.Status != http.StatusNotFound {
		t.Errorf("%d", resp.Status)
	}
}

func TestForgetRouteRemovesOthersAndKeepsTheOwnersDevices(t *testing.T) {
	r := devRig(t)
	_, _ = call(t, r, http.MethodPost, DevicesPath, `{"address":"3a:91:5c:d2:7e:44","label":"my phone"}`)
	if resp := r.post(t, `{"adapter":"wlx00","hours":1}`); resp.Status != http.StatusAccepted {
		t.Fatalf("%d %s", resp.Status, resp.Body)
	}
	waitFor(t, "a window", func() bool { return len(r.state(t).Windows) >= 1 })
	_, _ = call(t, r, http.MethodDelete, ListenPath, "")
	if resp, _ := call(t, r, http.MethodPost, ForgetPath, ""); resp.Status != http.StatusOK {
		t.Fatalf("%d %s", resp.Status, resp.Body)
	}
	resp, _ := call(t, r, http.MethodGet, DevicesPath, "")
	var list struct {
		Devices []OwnerDevice `json:"devices"`
	}
	_ = json.Unmarshal(resp.Body, &list)
	if len(list.Devices) != 1 {
		t.Errorf("the owner's device must stay: %s", resp.Body)
	}
}

func TestAHeardDeviceIsMarkedAsMineBySortIdWithoutAnAddress(t *testing.T) {
	r := devRig(t)
	rec, store, _ := r.s.recorder(r.s.h)
	cycleOf(t, rec, r.now, data(neighbour))
	var v AirDevices
	getQ(t, r, AirDevicesPath, "", &v)
	if len(v.Devices) != 1 {
		t.Fatalf("one neighbour expected: %+v", v)
	}
	id := v.Devices[0].ID
	path := "/admin/api/radio/air/mine/" + id
	resp, err := r.s.Handle(context.Background(), stacksdk.Request{Method: http.MethodPost, Path: path, Params: map[string]string{"id": id}, Body: []byte(`{"label":"hall plug"}`)})
	if err != nil || resp.Status != http.StatusCreated {
		t.Fatalf("%+v %v", resp, err)
	}
	var owner bool
	var label string
	_ = store.db.QueryRow(`SELECT owner, label FROM devices WHERE lower(substr(hex(tag),1,8)) = ?`, id).Scan(&owner, &label)
	if !owner || label != "hall plug" {
		t.Errorf("owner=%v label=%q", owner, label)
	}
	live := liveOf(t, r)
	for _, d := range live.Devices {
		if d.ID == id && (!d.Mine || d.Label != "hall plug") {
			t.Errorf("the radar should show it as mine: %+v", d)
		}
	}
	// A second try finds nothing to mark, an unknown id is refused, and a name is needed.
	again, _ := r.s.Handle(context.Background(), stacksdk.Request{Method: http.MethodPost, Path: path, Params: map[string]string{"id": id}, Body: []byte(`{"label":"x"}`)})
	unknown, _ := r.s.Handle(context.Background(), stacksdk.Request{Method: http.MethodPost, Path: "/admin/api/radio/air/mine/deadbeef", Params: map[string]string{"id": "deadbeef"}, Body: []byte(`{"label":"x"}`)})
	if again.Status != http.StatusNotFound || unknown.Status != http.StatusNotFound {
		t.Errorf("already mine %d, unknown %d", again.Status, unknown.Status)
	}
	noName, _ := r.s.Handle(context.Background(), stacksdk.Request{Method: http.MethodPost, Path: path, Params: map[string]string{"id": id}, Body: []byte(`{"label":""}`)})
	if noName.Status != http.StatusBadRequest {
		t.Errorf("a name is needed: %d", noName.Status)
	}
}

func TestAMarkFromTheRadarEndsWhenTheNeighboursAreForgotten(t *testing.T) {
	r := devRig(t)
	rec, store, _ := r.s.recorder(r.s.h)
	cycleOf(t, rec, r.now, data(neighbour))
	tags, _ := store.NeighbourTags(context.Background(), "")
	if len(tags) != 1 {
		t.Fatalf("%d", len(tags))
	}
	if err := store.MarkTagOwner(context.Background(), tags[0], "x", ""); err != nil {
		t.Fatal(err)
	}
	if err := rec.ForgetNeighbours(context.Background()); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = store.db.QueryRow(`SELECT count(*) FROM devices`).Scan(&n)
	if n != 0 {
		t.Errorf("a mark has no sealed address to tag again, so it ends with the rotation; %d rows remain", n)
	}
}
