package radio

import (
	"bytes"
	"context"
	"encoding/hex"
	"net/http"
	"slices"
	"testing"

	"llm-proxy-stacks/sdk"
)

// Before anything was recorded the report says the sensor has not listened and names no device: "not heard" is never "not there".
// The words the report carries are the SDK's: a kind, ring or band the stack adds must be added there too, or the core drops it.
func TestTheReportsWordsAreTheSDKs(t *testing.T) {
	for _, k := range append(slices.Clone(DeviceKinds), DeviceUnknown) {
		if !contains(stacksdk.AirKinds, string(k)) {
			t.Errorf("kind %q is not in stacksdk.AirKinds", k)
		}
	}
	for _, ring := range []string{RingRoom, RingHouse, RingNearby, RingFar} {
		if !contains(stacksdk.AirRings, ring) {
			t.Errorf("ring %q is not in stacksdk.AirRings", ring)
		}
	}
	for _, b := range []string{band24, band5} {
		if !contains(stacksdk.AirBands, b) {
			t.Errorf("band %q is not in stacksdk.AirBands", b)
		}
	}
}

func TestBandOfAChannel(t *testing.T) {
	for c, want := range map[int]string{0: "", 1: band24, 14: band24, 36: band5, 165: band5} {
		if got := bandOf(c); got != want {
			t.Errorf("channel %d: %q, want %q", c, got, want)
		}
	}
	if got := bandsOf([]int{36, 1, 6}); len(got) != 2 || got[0] != band24 || got[1] != band5 {
		t.Errorf("%v", got)
	}
}

func TestAirReportBeforeAnyListeningSaysItHasNotListened(t *testing.T) {
	r := devRig(t)
	got, err := r.s.Air(context.Background())
	if err != nil || got.Listened || len(got.Devices) != 0 {
		t.Fatalf("%+v %v", got, err)
	}
	off := newRig(t, `{"enabled":false}`, okWindow)
	if _, err := off.s.Air(context.Background()); stacksdk.CodeOf(err) != stacksdk.CodeUnavailable {
		t.Errorf("a stack switched off is unavailable, which the core reads as off: %v", err)
	}
}

// The report carries the devices the inventory gave by tag, and only those, with fixed values; a neighbour heard just as well is not in it.
func TestAirReportCarriesOnlyTheInventorysDevicesAsFixedValues(t *testing.T) {
	r := devRig(t)
	ctx := context.Background()
	rec, store, _ := r.s.recorder(r.s.h)
	confirmHomeAP(t, store, rec, false)
	rec.SetLAN(fakeLAN{neighbour: {Name: "Kitchen iPad", Presence: stacksdk.PresenceOnline}})
	for range 3 {
		cycleOf(t, rec, r.now, data(macAP), apBeacon(macAP, "home"), dataTo(macAP, neighbour), dataTo(neighbourAP, mineMAC))
	}
	got, err := r.s.Air(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Listened || got.LastCycle.IsZero() {
		t.Errorf("three cycles were listened: %+v", got)
	}
	want, _ := fakeVault{}.Tag(ctx, purpose(tagDevice, 0), neighbour[:])
	if len(got.Devices) != 1 || !bytes.Equal(got.Devices[0].Tag, want) {
		t.Fatalf("only the inventory's device: %+v", got.Devices)
	}
	f := got.Devices[0]
	if f.Heard.IsZero() || f.Band == "" || f.Ring == "" || !f.OnYourWiFi || f.Kind != string(DevicePhone) {
		t.Errorf("the fact: %+v", f)
	}
	for _, list := range []struct {
		v    string
		from []string
	}{{f.Band, stacksdk.AirBands}, {f.Ring, stacksdk.AirRings}, {f.Kind, stacksdk.AirKinds}} {
		if list.v != "" && !contains(list.from, list.v) {
			t.Errorf("%q is not one of the SDK's fixed words %v", list.v, list.from)
		}
	}
}

// The core links a device to its Radio page by the first four bytes of the fact's tag (devicejoin pageOf): that id must open the device's page.
func TestAirFactTagOpensTheDevicePage(t *testing.T) {
	r := devRig(t)
	rec, store, _ := r.s.recorder(r.s.h)
	confirmHomeAP(t, store, rec, false)
	rec.SetLAN(fakeLAN{neighbour: {Name: "Kitchen iPad", Presence: stacksdk.PresenceOnline}})
	for range 3 {
		cycleOf(t, rec, r.now, data(macAP), apBeacon(macAP, "home"), dataTo(macAP, neighbour))
	}
	got, err := r.s.Air(context.Background())
	if err != nil || len(got.Devices) != 1 {
		t.Fatalf("%+v %v", got, err)
	}
	if resp := callID(t, r, http.MethodGet, DeviceViewPath, hex.EncodeToString(got.Devices[0].Tag[:4]), ""); resp.Status != http.StatusOK {
		t.Errorf("the page the core links to: %d %s", resp.Status, resp.Body)
	}
}

// The presence the core gave reaches the owner's device on My devices, and nothing is said when the core said nothing.
func TestMyDevicesCarryTheNetworksPresence(t *testing.T) {
	r := devRig(t)
	ctx := context.Background()
	rec, _, _ := r.s.recorder(r.s.h)
	if err := rec.AddOwnerDevice(ctx, mineMAC, "phone", t0); err != nil {
		t.Fatal(err)
	}
	var v PresenceView
	getQ(t, r, PresencePath, "", &v)
	if len(v.Devices) != 1 || v.Devices[0].Network != stacksdk.PresenceUnknown {
		t.Fatalf("with no inventory nothing is said: %+v", v.Devices)
	}
	rec.SetLAN(fakeLAN{mineMAC: {Name: "Phone", Presence: stacksdk.PresenceOnline}})
	getQ(t, r, PresencePath, "", &v)
	if v.Devices[0].Network != stacksdk.PresenceOnline {
		t.Errorf("the network sees it now: %+v", v.Devices[0])
	}
	rec.SetLAN(fakeLAN{mineMAC: {Name: "Phone", Presence: "gone"}})
	var again PresenceView
	getQ(t, r, PresencePath, "", &again)
	if again.Devices[0].Network != stacksdk.PresenceUnknown {
		t.Errorf("a word the SDK does not know is dropped: %+v", again.Devices[0].Network)
	}
}

// A device of the inventory that asked to join a network that is not yours is offered to the nightly report by its tag, and a neighbour's
// device is not.
func TestFindingsOfferAForeignJoinByTheInventorysTag(t *testing.T) {
	r := devRig(t)
	ctx := context.Background()
	rec, _, _ := r.s.recorder(r.s.h)
	if err := rec.AddOwnerDevice(ctx, mineMAC, "phone", t0); err != nil {
		t.Fatal(err)
	}
	ownAP(t, r, 9, "home", "wpa2", "capable", true)
	r.now = t0
	record(t, rec, assocTo(mineMAC, neighbourAP), t0)
	got, err := r.s.Findings(ctx)
	if err != nil || len(got) != 0 {
		t.Fatalf("a device the inventory did not give is never offered: %+v %v", got, err)
	}
	rec.SetLAN(fakeLAN{mineMAC: {Name: "Phone"}})
	got, err = r.s.Findings(ctx)
	want, _ := fakeVault{}.Tag(ctx, purpose(tagDevice, 0), mineMAC[:])
	if err != nil || len(got) != 1 || got[0].ID != findingForeign || !bytes.Equal(got[0].Device, want) || got[0].Count != 1 || got[0].Subject.IsValid() {
		t.Fatalf("%+v %v", got, err)
	}
	off := newRig(t, `{"enabled":false}`, okWindow)
	if got, err := off.s.Findings(ctx); err != nil || len(got) != 0 {
		t.Errorf("a stack switched off offers nothing: %+v %v", got, err)
	}
	if err := r.s.Manifest().Validate(); err != nil || !r.s.Manifest().ReportsAir {
		t.Errorf("the manifest declares the report: %v", err)
	}
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
