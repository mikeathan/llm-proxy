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
	view := ViewOf(inv, later)

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
	view := ViewOf(inv, t0.Add(time.Hour))
	if len(view.Devices) != 2 || view.Devices[0].MAC != macOther {
		t.Errorf("what needs a look is listed first: %+v", view.Devices)
	}
}

func TestViewOf_GatewayChangedIsShown(t *testing.T) {
	inv := Observe(models.DeviceInventory{}, table("192.168.1.1", nb("192.168.1.1", macRouter)), anywhere, t0)
	inv = Observe(inv, table("192.168.1.1", nb("192.168.1.1", macOther)), anywhere, t0.Add(time.Hour))
	if v := ViewOf(inv, t0.Add(time.Hour)); !v.Gateway.Changed {
		t.Errorf("gateway = %+v", v.Gateway)
	}
}

func TestViewOf_NamesTheMakerOfAKnownCardAndNothingForARandomizedOne(t *testing.T) {
	// e8:0a:b9 is a Cisco prefix in the IEEE registry; 92:... has the locally administered bit set, so says nothing.
	const cisco, private = "e8:0a:b9:12:34:56", "92:4b:fe:30:5a:61"
	inv := Observe(models.DeviceInventory{}, table("", nb("192.168.1.30", cisco), nb("192.168.1.31", private)), anywhere, t0)
	for _, d := range ViewOf(inv, t0).Devices {
		switch d.MAC {
		case cisco:
			if d.Vendor != "Cisco Systems, Inc" {
				t.Errorf("vendor = %q", d.Vendor)
			}
		case private:
			if d.Vendor != "" {
				t.Errorf("a randomized address has no vendor, got %q", d.Vendor)
			}
		}
	}
}
