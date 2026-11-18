package devicejoin

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"net"
	"testing"
	"time"

	"llm-proxy-stacks/sdk"

	"llm-proxy/models"
)

var t0 = time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)

// tagOf is a vault-like tag: the same purpose and value give the same bytes, another purpose another.
func tagOf(_ context.Context, purpose string, value []byte) ([]byte, error) {
	m := hmac.New(sha256.New, []byte(purpose))
	m.Write(value)
	return m.Sum(nil), nil
}

func stackTag(t *testing.T, purpose, mac string) []byte {
	t.Helper()
	hw, err := net.ParseMAC(mac)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := tagOf(context.Background(), purpose, hw)
	return b
}

type rig struct {
	inv      models.DeviceInventory
	report   stacksdk.AirReport
	err      error
	purposes []string
	now      time.Time
	pulls    int
}

func (r *rig) join(withAir bool) *Join {
	d := Deps{
		Inventory: func() (models.DeviceInventory, error) { return r.inv, nil },
		Purposes:  func() []string { return r.purposes },
		Tag:       tagOf,
		Now:       func() time.Time { return r.now },
	}
	if withAir {
		d.Air = func(context.Context) (stacksdk.AirReport, error) { r.pulls++; return r.report, r.err }
	}
	return New(d)
}

func newRig() *rig {
	return &rig{
		inv:      models.DeviceInventory{Devices: []models.NetworkDevice{{MAC: "AA:BB:CC:00:00:10", IP: "192.168.1.10"}, {MAC: "aa:bb:cc:00:00:11", IP: "192.168.1.11"}}},
		purposes: []string{"radio_device_e1"},
		now:      t0,
	}
}

// Only the facts whose tag is an inventory device's are kept, under the purpose the stack asked for; a neighbour's tag matches nothing.
func TestTheJoinKeepsOnlyTheInventorysDevices(t *testing.T) {
	r := newRig()
	r.report = stacksdk.AirReport{Listened: true, LastCycle: t0.Add(-time.Minute), Bands: []string{"2G", "nope"}, Devices: []stacksdk.AirFact{
		{Tag: stackTag(t, "radio_device_e1", "aa:bb:cc:00:00:10"), Heard: t0.Add(-2 * time.Minute), Band: "5G", Ring: "house", Kind: "phone", OnYourWiFi: true},
		{Tag: stackTag(t, "radio_device_e1", "de:ad:be:ef:00:01"), Heard: t0, Ring: "room"}, // a neighbour
		{Tag: stackTag(t, "radio_device", "aa:bb:cc:00:00:11"), Heard: t0, Ring: "room"},    // a purpose the stack did not ask for
	}}
	v := r.join(true).View(context.Background())
	if v.State != models.AirListening || v.LastCycle.IsZero() || len(v.Bands) != 1 {
		t.Fatalf("%+v", v)
	}
	if len(v.ByMAC) != 1 {
		t.Fatalf("only the inventory's device under the asked purpose: %+v", v.ByMAC)
	}
	d, ok := Of(v, "aa:bb:cc:00:00:10")
	if !ok || d.Ring != "house" || d.Band != "5G" || d.Kind != "phone" || !d.OnYourWiFi {
		t.Errorf("%+v %v", d, ok)
	}
}

// A word the SDK does not list, a time from the future, and a place for a device not heard lately are all dropped.
func TestTheJoinDropsWhatItCannotVouchFor(t *testing.T) {
	r := newRig()
	tag := stackTag(t, "radio_device_e1", "aa:bb:cc:00:00:10")
	r.report = stacksdk.AirReport{Listened: true, Devices: []stacksdk.AirFact{{Tag: tag, Heard: t0.Add(time.Hour), Band: "5G", Ring: "under the sofa", Kind: "<b>"}}}
	d, _ := Of(r.join(true).View(context.Background()), "aa:bb:cc:00:00:10")
	if !d.Heard.IsZero() || d.Band != "" || d.Ring != "" || d.Kind != "" {
		t.Errorf("%+v", d)
	}
}

// Each side off is a state, never a failure that hides the page: no stack is off, a stopped stack is off, a broken one could not be read, and
// a stack that has not listened lately says nothing of anyone.
func TestEachSideOffIsAState(t *testing.T) {
	r := newRig()
	if v := r.join(false).View(context.Background()); v.State != models.AirOff {
		t.Errorf("no stack: %+v", v)
	}
	r.err = stacksdk.Errorf(stacksdk.CodeUnavailable, "not running")
	if v := r.join(true).View(context.Background()); v.State != models.AirOff {
		t.Errorf("a stopped stack: %+v", v)
	}
	r.err = errors.New("broken")
	if v := r.join(true).View(context.Background()); v.State != models.AirUnreadable {
		t.Errorf("a broken stack: %+v", v)
	}
	r.err = nil
	r.report = stacksdk.AirReport{Listened: false}
	if v := r.join(true).View(context.Background()); v.State != models.AirNotListening {
		t.Errorf("not listening: %+v", v)
	}
	r.inv = models.DeviceInventory{}
	j := New(Deps{Inventory: func() (models.DeviceInventory, error) { return models.DeviceInventory{}, errors.New("x") },
		Air: func(context.Context) (stacksdk.AirReport, error) { return stacksdk.AirReport{Listened: true}, nil }, Now: func() time.Time { return t0 }})
	if v := j.View(context.Background()); v.State != models.AirUnreadable {
		t.Errorf("an unreadable inventory: %+v", v)
	}
}

// The report is pulled at most once a minute however often the page asks.
func TestTheJoinIsReadAtMostOnceAMinute(t *testing.T) {
	r := newRig()
	j := r.join(true)
	j.View(context.Background())
	j.View(context.Background())
	r.now = r.now.Add(30 * time.Second)
	j.View(context.Background())
	if r.pulls != 1 {
		t.Errorf("pulled %d times within a minute", r.pulls)
	}
	r.now = r.now.Add(time.Minute)
	j.View(context.Background())
	if r.pulls != 2 {
		t.Errorf("pulled %d times after a minute", r.pulls)
	}
}

// The new-device note says where the air heard the new devices, by counts only, and nothing when the sensor did not listen.
func TestNewDeviceNote(t *testing.T) {
	heard := func(ring string, ago time.Duration) models.DeviceAir {
		return models.DeviceAir{Heard: t0.Add(-ago), Ring: ring}
	}
	v := models.AirView{State: models.AirListening, ByMAC: map[string]models.DeviceAir{
		"aa:bb:cc:00:00:01": heard("house", time.Minute), "aa:bb:cc:00:00:02": heard("far", time.Minute), "aa:bb:cc:00:00:03": heard("room", 3*time.Hour),
	}}
	cases := []struct {
		macs []string
		want string
	}{
		{[]string{"AA:BB:CC:00:00:01"}, "Radio heard it over the air inside the house."},
		{[]string{"aa:bb:cc:00:00:02"}, "Radio heard it over the air only from outside the house."},
		{[]string{"aa:bb:cc:00:00:03"}, "Radio has not heard it over the air lately."},
		{[]string{"aa:bb:cc:00:00:03", "aa:bb:cc:00:00:09"}, "Radio has not heard them over the air lately."},
		{[]string{"aa:bb:cc:00:00:01", "aa:bb:cc:00:00:02", "aa:bb:cc:00:00:03"}, "Radio heard 1 of them inside the house and 1 of them only from outside the house."},
	}
	for _, c := range cases {
		if got := NewDeviceNote(v, c.macs, t0); got != c.want {
			t.Errorf("%v: %q", c.macs, got)
		}
	}
	v.State = models.AirNotListening
	if got := NewDeviceNote(v, []string{"aa:bb:cc:00:00:01"}, t0); got != "" {
		t.Errorf("a sensor that did not listen says nothing: %q", got)
	}
}

// A stack's tag maps back to the inventory device under each purpose the stack asked for, and to nothing else.
func TestIndexMapsTheStacksTagsBack(t *testing.T) {
	r := newRig()
	r.purposes = []string{"radio_device_e2", "radio_device_e1"}
	idx, err := r.join(false).Index(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range r.purposes {
		if d, ok := idx[string(stackTag(t, p, "aa:bb:cc:00:00:11"))]; !ok || d.IP != "192.168.1.11" {
			t.Errorf("%s: %+v", p, d)
		}
	}
	if _, ok := idx[string(stackTag(t, "radio_ap", "aa:bb:cc:00:00:11"))]; ok {
		t.Error("a purpose the stack did not ask for maps nothing")
	}
}

// A report that cannot be read is told once per cause, and again only after a read worked, so a broken stack is in the log, not silent.
func TestTheJoinTellsAFailureOncePerCause(t *testing.T) {
	r := newRig()
	var told []error
	j := New(Deps{Inventory: func() (models.DeviceInventory, error) { return r.inv, nil }, Now: func() time.Time { return r.now },
		Air:     func(context.Context) (stacksdk.AirReport, error) { return r.report, r.err },
		OnError: func(err error) { told = append(told, err) }})
	r.err = errors.New("broken")
	for range 3 {
		j.View(context.Background())
		r.now = r.now.Add(2 * time.Minute)
	}
	if len(told) != 1 {
		t.Fatalf("told %d times: %v", len(told), told)
	}
	r.err = nil
	j.View(context.Background())
	r.now = r.now.Add(2 * time.Minute)
	r.err = errors.New("broken")
	j.View(context.Background())
	if len(told) != 2 {
		t.Errorf("after a read that worked the same cause is told again: %v", told)
	}
}
