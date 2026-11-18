package radio

import (
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"llm-proxy-stacks/sdk"
)

func callID(t *testing.T, r *listenRig, method, pattern, id, body string) stacksdk.Response {
	t.Helper()
	resp, err := r.s.Handle(context.Background(), stacksdk.Request{Method: method, Path: strings.Replace(pattern, "{id}", id, 1), Params: map[string]string{"id": id}, Body: []byte(body)})
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func shortOf(m MAC) string {
	tag, _ := fakeVault{}.Tag(context.Background(), tagDevice, m[:])
	return shortID(tag)
}

// fakeLAN is the core's inventory with the given addresses and names, tagged the way the core tags them.
type fakeLAN map[MAC]stacksdk.LANDevice

func (l fakeLAN) Devices(ctx context.Context, purpose string) ([]stacksdk.LANDevice, error) {
	var out []stacksdk.LANDevice
	for m, d := range l {
		d.Tag, _ = fakeVault{}.Tag(ctx, purpose, m[:])
		out = append(out, d)
	}
	return out, nil
}

// apBeacon is a beacon from ap on channel 6 announcing Wi-Fi 6.
func apBeacon(ap MAC, name string) []byte {
	return rtap(cat(hdr(0x80, 0, macBroad, ap, ap), make([]byte, 12), ie(0, []byte(name)...), ie(3, 6), ie(255, 35)))
}

func dataTo(ap, from MAC) []byte {
	return rtap(cat(hdr(0x88, 0x01, ap, from, ap), []byte{0, 0}, make([]byte, 10)))
}

func TestTheDevicePageNamesADeviceByItsNetworkAndRouter(t *testing.T) {
	r := devRig(t)
	rec, store, _ := r.s.recorder(r.s.h)
	confirmHomeAP(t, store, rec, false)
	rec.SetLAN(fakeLAN{neighbour: {Name: "Kitchen iPad"}})
	for range 14 {
		cycleOf(t, rec, r.now, data(macAP), apBeacon(macAP, "home"), dataTo(macAP, neighbour))
	}
	resp := callID(t, r, http.MethodGet, DeviceViewPath, shortOf(neighbour), "")
	if resp.Status != http.StatusOK {
		t.Fatalf("%d %s", resp.Status, resp.Body)
	}
	var v DeviceView
	if err := json.Unmarshal(resp.Body, &v); err != nil {
		t.Fatal(err)
	}
	id := v.Device.Ident
	if id == nil || id.LANName != "Kitchen iPad" || !id.OnYourWiFi || !id.NetYours || !id.LikelyYours() {
		t.Fatalf("the device's identity: %+v", id)
	}
	if v.Uses == nil || v.Uses.ID != shortOf(macAP) || v.Uses.Network != "home" {
		t.Errorf("it uses your access point: %+v", v.Uses)
	}
	if len(v.Chances) == 0 {
		t.Errorf("the model's chances should be given")
	}
	if v.Device.Mine {
		t.Error("evidence is shown, never turned into ownership")
	}
	if callID(t, r, http.MethodGet, DeviceViewPath, "deadbeef", "").Status != http.StatusNotFound {
		t.Error("an unknown device is not found")
	}
}

func TestNetworksAroundGroupARoutersRadiosAndTheirClients(t *testing.T) {
	r := devRig(t)
	rec, store, _ := r.s.recorder(r.s.h)
	confirmHomeAP(t, store, rec, false)
	twin := MAC{0x02, 0, 0, 0, 0, 0x03} // the same block as macAP (02:00:00:00:00:01): one router
	far := MAC{0x5c, 0x11, 0x22, 0x33, 0x44, 0x50}
	for range 14 {
		cycleOf(t, rec, r.now, data(macAP), apBeacon(macAP, "home"), data(twin), apBeacon(twin, "home-guest"), dataTo(macAP, neighbour),
			data(far), apBeacon(far, "Other"), dataTo(far, neighbourAP), probeWith(mineMAC, ie(0)))
	}
	var v AroundNetworks
	get(t, r, AroundNetworksPath, &v)
	if len(v.Groups) != 2 {
		t.Fatalf("two routers: %+v", v.Groups)
	}
	home := v.Groups[0]
	if !home.Yours || len(home.Radios) != 2 || len(home.Clients) != 1 || home.Clients[0].ID != shortOf(neighbour) {
		t.Errorf("your router first, with both radios and its client: %+v", home)
	}
	if other := v.Groups[1]; other.Yours || len(other.Clients) != 1 || other.Clients[0].ID != shortOf(neighbourAP) {
		t.Errorf("the other router and its client: %+v", other)
	}
	if len(v.Unconnected) != 1 || v.Unconnected[0].ID != shortOf(mineMAC) {
		t.Errorf("a device seen on no network: %+v", v.Unconnected)
	}
}

func TestAskListsLikelyYoursFirstAndNeverOneYouAnswered(t *testing.T) {
	r := devRig(t)
	rec, store, _ := r.s.recorder(r.s.h)
	confirmHomeAP(t, store, rec, false)
	for range 14 {
		cycleOf(t, rec, r.now, data(macAP), apBeacon(macAP, "home"), dataTo(macAP, neighbour))
	}
	var v AskView
	get(t, r, AskPath, &v)
	if len(v.Devices) == 0 || v.Devices[0].ID != shortOf(neighbour) || v.Devices[0].Reason != "likely_yours" {
		t.Fatalf("%+v", v.Devices)
	}
	if resp := callID(t, r, http.MethodPut, ClassPath, shortOf(neighbour), `{"class":"personal"}`); resp.Status != http.StatusOK {
		t.Fatalf("%d %s", resp.Status, resp.Body)
	}
	get(t, r, AskPath, &v)
	for _, d := range v.Devices {
		if d.ID == shortOf(neighbour) {
			t.Error("a device the owner answered is not asked about again")
		}
	}
}

func TestTheOwnersKindAndClassStandAndCanBeHandedBack(t *testing.T) {
	r := devRig(t)
	rec, store, _ := r.s.recorder(r.s.h)
	for range 14 {
		cycleOf(t, rec, r.now, data(neighbour))
	}
	id := shortOf(neighbour)
	if resp := callID(t, r, http.MethodPut, KindPath, id, `{"kind":"tv"}`); resp.Status != http.StatusOK {
		t.Fatalf("%d %s", resp.Status, resp.Body)
	}
	if resp := callID(t, r, http.MethodPut, KindPath, id, `{"kind":"toaster"}`); resp.Status != http.StatusBadRequest {
		t.Errorf("an unknown kind is refused: %d", resp.Status)
	}
	if resp := callID(t, r, http.MethodPut, ClassPath, id, `{"class":"neighbour"}`); resp.Status != http.StatusOK {
		t.Fatalf("%d", resp.Status)
	}
	for range 3 {
		cycleOf(t, rec, r.now, data(neighbour))
	}
	row := rowOf(t, store, neighbour)
	if row.Class != "neighbour" || row.ClassVer != 0 || row.Kind != "tv" || row.KindBy != kindByOwner {
		t.Fatalf("the owner's answers stand: %+v", row)
	}
	if g := KindOf(row, nil); g.Kind != DeviceTV || g.Source != KindFromOwner {
		t.Errorf("kind %+v", g)
	}
	_ = callID(t, r, http.MethodPut, ClassPath, id, `{"class":""}`)
	cycleOf(t, rec, r.now, data(neighbour))
	if row := rowOf(t, store, neighbour); row.ClassVer == 0 {
		t.Errorf("handed back, the classifier decides again: %+v", row)
	}
}

func TestAGuestNetworkCanBeMarkedAndCalibrationTakesADistance(t *testing.T) {
	r := devRig(t)
	rec, store, _ := r.s.recorder(r.s.h)
	confirmHomeAP(t, store, rec, false)
	apTag, _ := fakeVault{}.Tag(context.Background(), tagAP, macAP[:])
	if resp := callID(t, r, http.MethodPut, GuestPath, shortID(apTag)+strings.Repeat("0", 0), `{"guest":true}`); resp.Status != http.StatusNotFound {
		t.Errorf("a short id is not an access point's id: %d", resp.Status)
	}
	full := hexOf(apTag)
	if resp := callID(t, r, http.MethodPut, GuestPath, full, `{"guest":true}`); resp.Status != http.StatusOK {
		t.Fatalf("%d %s", resp.Status, resp.Body)
	}
	cycleOf(t, rec, r.now, data(macAP), apBeacon(macAP, "home"))
	rec.BeginCycle()
	record(t, rec, data(macAP), r.now)
	resp := callID(t, r, http.MethodPost, CalibratePath, shortOf(macAP), `{"metres":4}`)
	if resp.Status != http.StatusOK {
		t.Fatalf("your own access point, 4 m away, calibrates: %d %s", resp.Status, resp.Body)
	}
	var m DistanceModel
	_ = json.Unmarshal(resp.Body, &m)
	if !m.Calibrated || m.Ref5 <= -60 {
		t.Errorf("a -60 dBm reading at 4 m puts the one-metre signal above -60: %+v", m)
	}
	if resp := callID(t, r, http.MethodPost, CalibratePath, shortOf(macAP), `{"metres":300}`); resp.Status != http.StatusBadRequest {
		t.Errorf("a distance out of range is refused: %d", resp.Status)
	}
}

func hexOf(b []byte) string { return hex.EncodeToString(b) }

func TestADeviceOnYourNetworkIsExplainedWithTheModelThatSortedIt(t *testing.T) {
	r := devRig(t)
	rec, store, _ := r.s.recorder(r.s.h)
	rec.SetLAN(fakeLAN{neighbour: {Name: "Kitchen iPad"}})
	for range 14 {
		cycleOf(t, rec, r.now, dataAt(neighbour, -85)) // weak: alone it would read as a neighbour's
	}
	var v DeviceView
	resp := callID(t, r, http.MethodGet, DeviceViewPath, shortOf(neighbour), "")
	if err := json.Unmarshal(resp.Body, &v); err != nil {
		t.Fatal(err)
	}
	if v.Chances["neighbour"] != 0 || v.Chances["transient"] != 0 {
		t.Errorf("on your network, it is never a neighbour's or a passer-by: %v", v.Chances)
	}
	best, bp := "", -1
	for c, p := range v.Chances {
		if p > bp {
			best, bp = c, p
		}
	}
	if row := rowOf(t, store, neighbour); best != row.Class {
		t.Errorf("the page's top chance %q must be the stored class %q", best, row.Class)
	}
}

func TestNotMineRecordsAPasserByAsOneAndNotAsANeighbour(t *testing.T) {
	r := devRig(t)
	rec, store, _ := r.s.recorder(r.s.h)
	for i := range 14 {
		frames := [][]byte{data(macAP)}
		if i == 3 {
			frames = append(frames, dataAt(neighbour, -72))
		}
		cycleOf(t, rec, r.now, frames...)
	}
	if resp := callID(t, r, http.MethodPut, ClassPath, shortOf(neighbour), `{"class":"not_mine"}`); resp.Status != http.StatusOK {
		t.Fatalf("%d %s", resp.Status, resp.Body)
	}
	if row := rowOf(t, store, neighbour); row.Class != string(ClassTransient) || row.ClassVer != 0 {
		t.Errorf("heard once, not mine: a passer-by, as the owner's word: %+v", row)
	}
}

func TestAnInventoryNameNeverReachesTheFile(t *testing.T) {
	r := devRig(t)
	rec, store, _ := r.s.recorder(r.s.h)
	rec.SetLAN(fakeLAN{neighbour: {Name: "KitchenIPadZ"}})
	for range 14 {
		cycleOf(t, rec, r.now, dataAt(neighbour, -50))
	}
	var v LiveView
	get(t, r, LivePath, &v)
	if resp := callID(t, r, http.MethodPost, MarkMinePath, shortOf(neighbour), `{"label":"Roku TV"}`); resp.Status != http.StatusCreated {
		t.Fatalf("%d %s", resp.Status, resp.Body)
	}
	var path string
	if err := store.db.QueryRow(`PRAGMA database_list`).Scan(new(int), new(string), &path); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{path, path + "-wal"} {
		raw, err := os.ReadFile(f)
		if err == nil && bytes.Contains(raw, []byte("KitchenIPadZ")) {
			t.Errorf("%s holds the inventory's name", f)
		}
	}
}
