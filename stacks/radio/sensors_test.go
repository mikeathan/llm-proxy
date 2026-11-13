package radio

import (
	"io/fs"
	"testing"
	"testing/fstest"
)

// usbWifi adds a USB Wi-Fi adapter to a fake /sys the way the kernel lays it out: class/net/<name> is a link into the device tree.
func usbWifi(m fstest.MapFS, name, driver, vendor, product, mode, state string) {
	dev := "devices/pci0000:00/0000:00:14.0/usb1/1-2"
	iface := dev + "/1-2:1.0"
	m["class/net/"+name] = &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("../../" + iface + "/net/" + name)}
	m[iface+"/net/"+name+"/phy80211/index"] = &fstest.MapFile{Data: []byte("1\n")}
	m[iface+"/net/"+name+"/type"] = &fstest.MapFile{Data: []byte(mode + "\n")}
	m[iface+"/net/"+name+"/operstate"] = &fstest.MapFile{Data: []byte(state + "\n")}
	m[iface+"/driver"] = &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("../../../../../../bus/usb/drivers/" + driver)}
	m[dev+"/idVendor"] = &fstest.MapFile{Data: []byte(vendor + "\n")}
	m[dev+"/idProduct"] = &fstest.MapFile{Data: []byte(product + "\n")}
}

// pciWifi adds the built-in card, the one the computer is usually connected through.
func pciWifi(m fstest.MapFS, name, state string) {
	iface := "devices/pci0000:00/0000:00:1c.0/0000:02:00.0"
	m["class/net/"+name] = &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("../../" + iface + "/net/" + name)}
	m[iface+"/net/"+name+"/phy80211/index"] = &fstest.MapFile{Data: []byte("0\n")}
	m[iface+"/net/"+name+"/type"] = &fstest.MapFile{Data: []byte("1\n")}
	m[iface+"/net/"+name+"/operstate"] = &fstest.MapFile{Data: []byte(state + "\n")}
	m[iface+"/driver"] = &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("../../../../bus/pci/drivers/iwlwifi")}
}

func byID(t *testing.T, sensors []Sensor, id string) Sensor {
	t.Helper()
	for _, s := range sensors {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("no sensor %q in %+v", id, sensors)
	return Sensor{}
}

func capability(t *testing.T, s Sensor, id string) Capability {
	t.Helper()
	for _, c := range s.Capabilities {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("sensor %q has no capability %q: %+v", s.ID, id, s.Capabilities)
	return Capability{}
}

func TestDiscover_NothingThereIsNotAnError(t *testing.T) {
	for name, sys := range map[string]fs.FS{
		"no sys at all (macOS)": fstest.MapFS{},
		"no wireless adapter":   fstest.MapFS{"class/net/eth0": &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("../../devices/pci0000:00/0000:00:1f.6/net/eth0")}, "devices/pci0000:00/0000:00:1f.6/net/eth0/type": &fstest.MapFile{Data: []byte("1\n")}},
	} {
		snap := Discover(sys)
		if len(snap.Sensors) != 0 || snap.Reason == "" {
			t.Errorf("%s: want no sensors and a plain reason, got %+v", name, snap)
		}
	}
}

func TestDiscover_DescribesAUsbAdapterAndKeepsTheBuiltInCardOut(t *testing.T) {
	sys := fstest.MapFS{}
	usbWifi(sys, "wlx00", "rtw_8822bu", "0bda", "b812", "1", "down")
	pciWifi(sys, "wlp2s0", "up")

	snap := Discover(sys)
	if len(snap.Sensors) != 2 {
		t.Fatalf("want both radios listed, got %+v", snap.Sensors)
	}
	usb := byID(t, snap.Sensors, "wlx00")
	if usb.Kind != KindWifi || usb.Bus != "usb" || usb.Driver != "rtw_8822bu" || usb.USBID != "0bda:b812" || usb.Mode != ModeManaged {
		t.Errorf("usb adapter = %+v", usb)
	}
	if !usb.Eligible {
		t.Errorf("an idle USB adapter is eligible: %+v", usb)
	}
	card := byID(t, snap.Sensors, "wlp2s0")
	if card.Bus != "pci" || card.Eligible || card.Why == "" {
		t.Errorf("the built-in card is listed but never eligible, with the reason: %+v", card)
	}
}

func TestDiscover_AUsbAdapterThisComputerIsConnectedThroughIsNotEligible(t *testing.T) {
	sys := fstest.MapFS{}
	usbWifi(sys, "wlx00", "rtw_8822bu", "0bda", "b812", "1", "up")
	s := byID(t, Discover(sys).Sensors, "wlx00")
	if s.Eligible || s.Why == "" {
		t.Errorf("an adapter in use for the connection must not be offered for capture: %+v", s)
	}
}

func TestDiscover_MonitorModeIsRecognisedFromTheLinkType(t *testing.T) {
	sys := fstest.MapFS{}
	usbWifi(sys, "wlx00", "rtw_8822bu", "0bda", "b812", "803", "unknown")
	s := byID(t, Discover(sys).Sensors, "wlx00")
	if s.Mode != ModeMonitor || !s.Eligible {
		t.Errorf("link type 803 is monitor mode: %+v", s)
	}
}

// The capability list is the future-work list: what is used, what is refused by design, what is not built yet.
func TestDiscover_ListsWhatIsUsedAndWhatIsNot(t *testing.T) {
	sys := fstest.MapFS{}
	usbWifi(sys, "wlx00", "mt7601u", "148f", "7601", "1", "down")
	s := byID(t, Discover(sys).Sensors, "wlx00")
	for id, want := range map[string]Use{
		"discovery":       UseUsed,
		"frame_injection": UseByDesign,
		"monitor_capture": UseNotBuilt,
		"he_decode":       UseNotBuilt,
		"ranging":         UseNotBuilt,
	} {
		if got := capability(t, s, id).Use; got != want {
			t.Errorf("capability %q use = %q, want %q", id, got, want)
		}
	}
	for _, c := range s.Capabilities {
		if c.Title == "" || c.Note == "" {
			t.Errorf("a capability is shown with its words: %+v", c)
		}
	}
}

func TestDiscover_BluetoothAndIEEE802154AreFoundWithoutADriver(t *testing.T) {
	sys := fstest.MapFS{
		"class/bluetooth/hci0": &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("../../devices/pci0000:00/0000:00:14.0/usb1/1-10/1-10:1.0/bluetooth/hci0")},
		"devices/pci0000:00/0000:00:14.0/usb1/1-10/1-10:1.0/bluetooth/hci0/address": &fstest.MapFile{Data: []byte("aa:bb\n")},
		"class/net/wpan0":                &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("../../devices/virtual/net/wpan0")},
		"devices/virtual/net/wpan0/type": &fstest.MapFile{Data: []byte("804\n")},
	}
	snap := Discover(sys)
	bt := byID(t, snap.Sensors, "hci0")
	if bt.Kind != KindBluetooth || bt.Eligible {
		t.Errorf("bluetooth = %+v", bt)
	}
	if c := capability(t, bt, "ble_scan"); c.Use != UseNoDriver {
		t.Errorf("a found radio without a driver says so: %+v", c)
	}
	zb := byID(t, snap.Sensors, "wpan0")
	if zb.Kind != KindIEEE802154 || capability(t, zb, "zigbee_sniff").Use != UseNoDriver {
		t.Errorf("802.15.4 = %+v", zb)
	}
}

// Nothing a device says reaches this record: it is built from the system's own files, and a value that is not a plain token is dropped.
func TestDiscover_RefusesWordsThatAreNotPlainTokens(t *testing.T) {
	sys := fstest.MapFS{}
	usbWifi(sys, "wlx00", "bad driver\nname", "0bda", "zz zz", "1", "down")
	s := byID(t, Discover(sys).Sensors, "wlx00")
	if s.Driver != "" || s.USBID != "" {
		t.Errorf("a value that is not a plain token is dropped, got driver %q id %q", s.Driver, s.USBID)
	}
}
