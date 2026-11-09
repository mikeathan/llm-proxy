package netaudit

import (
	"testing"
	"time"

	"llm-proxy/models"
)

func TestViewOf_ShowsWhatTheScreenNeeds(t *testing.T) {
	inv := Observe(models.DeviceInventory{}, table("192.168.1.1", nb("192.168.1.1", macRouter), nb("192.168.1.20", macTV), nb("192.168.1.21", macTV)), anywhere, t0)
	later := t0.Add(time.Hour)
	inv = Observe(inv, table("192.168.1.1", nb("192.168.1.1", macRouter), nb("192.168.1.77", macOther)), anywhere, later)
	view := ViewOf(inv, later, nil)

	if !view.Baselined || view.Gateway.IP != "192.168.1.1" || view.Gateway.Changed {
		t.Errorf("gateway = %+v", view.Gateway)
	}
	var tv, stranger *DeviceView
	for i := range view.Devices {
		switch view.Devices[i].MAC {
		case macTV:
			tv = &view.Devices[i]
		case macOther:
			stranger = &view.Devices[i]
		}
	}
	if tv == nil || stranger == nil {
		t.Fatalf("devices = %+v", view.Devices)
	}
	if tv.New || !tv.Approved {
		t.Errorf("a baseline device is not new: %+v", tv)
	}
	if !stranger.New || stranger.Approved {
		t.Errorf("a device that appeared after the baseline is new: %+v", stranger)
	}
}

func TestViewOf_NewDevicesComeFirst(t *testing.T) {
	inv := Observe(models.DeviceInventory{}, table("", nb("192.168.1.20", macTV)), anywhere, t0)
	inv = Observe(inv, table("", nb("192.168.1.77", macOther)), anywhere, t0.Add(time.Hour))
	view := ViewOf(inv, t0.Add(time.Hour), nil)
	if len(view.Devices) != 2 || view.Devices[0].MAC != macOther {
		t.Errorf("what needs a look is listed first: %+v", view.Devices)
	}
}

func TestViewOf_GatewayChangedIsShown(t *testing.T) {
	inv := Observe(models.DeviceInventory{}, table("192.168.1.1", nb("192.168.1.1", macRouter)), anywhere, t0)
	inv = Observe(inv, table("192.168.1.1", nb("192.168.1.1", macOther)), anywhere, t0.Add(time.Hour))
	if v := ViewOf(inv, t0.Add(time.Hour), nil); !v.Gateway.Changed {
		t.Errorf("gateway = %+v", v.Gateway)
	}
}

// The registry of card makers lives in the Network stack, so the core is handed a lookup and shows whatever it answers; with none it
// shows no maker. (What the registry answers for a randomized address is the registry's own test.)
func TestViewOf_ShowsTheMakerTheLookupNamesAndNoneWithoutALookup(t *testing.T) {
	const known, other = "e8:0a:b9:12:34:56", "92:4b:fe:30:5a:61"
	inv := Observe(models.DeviceInventory{}, table("", nb("192.168.1.30", known), nb("192.168.1.31", other)), anywhere, t0)
	lookup := VendorLookup(func(mac string) string {
		if mac == known {
			return "Cisco Systems, Inc"
		}
		return ""
	})
	for _, d := range ViewOf(inv, t0, lookup).Devices {
		if want := map[string]string{known: "Cisco Systems, Inc", other: ""}[d.MAC]; d.Vendor != want {
			t.Errorf("%s: vendor = %q, want %q", d.MAC, d.Vendor, want)
		}
	}
	for _, d := range ViewOf(inv, t0, nil).Devices {
		if d.Vendor != "" {
			t.Errorf("with no lookup there is no maker, got %q", d.Vendor)
		}
	}
}
