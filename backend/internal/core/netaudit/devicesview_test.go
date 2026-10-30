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
