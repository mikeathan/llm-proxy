package radio

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"testing"
	"testing/fstest"
)

// withAddress gives an interface added by usbWifi or pciWifi its hardware address, where the kernel keeps it.
func withAddress(m fstest.MapFS, name, addr string) {
	link := string(m["class/net/"+name].Data)
	m[link[len("../../"):]+"/address"] = &fstest.MapFile{Data: []byte(addr + "\n")}
}

func TestHostRadiosListThisComputersOwnWiFiAddresses(t *testing.T) {
	m := fstest.MapFS{}
	pciWifi(m, "wlp2s0", "up")
	withAddress(m, "wlp2s0", "b8:27:eb:19:6d:a3")
	usbWifi(m, "wlx00", "rtw88_8822bu", "2357", "012e", "803", "up")
	withAddress(m, "wlx00", "00:00:00:00:00:00") // no address: skipped
	m["class/net/eth0"] = &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("../../devices/pci0000:00/0000:00:1f.6/net/eth0")}
	m["devices/pci0000:00/0000:00:1f.6/net/eth0/type"] = &fstest.MapFile{Data: []byte("1\n")}
	m["devices/pci0000:00/0000:00:1f.6/net/eth0/address"] = &fstest.MapFile{Data: []byte("02:11:22:33:44:55\n")}

	got := HostRadios(m)
	if len(got) != 1 || got[0].Iface != "wlp2s0" || got[0].MAC != neighbour {
		t.Fatalf("only the built-in card's address, the wired port is never heard over the air: %+v", got)
	}
	if len(HostRadios(fstest.MapFS{})) != 0 {
		t.Errorf("a computer without /sys has none")
	}
}

func TestTheDevicePageSaysAThisComputersOwnCardIsThisComputer(t *testing.T) {
	r := devRig(t)
	rec, store, _ := r.s.recorder(r.s.h)
	confirmHomeAP(t, store, rec, false)
	rec.SetHost([]HostRadio{{Iface: "wlp2s0", MAC: neighbour}})
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
	if id == nil || id.ThisComputer != "wlp2s0" || !id.LikelyYours() {
		t.Fatalf("the listening computer's own card: %+v", id)
	}
	if id.Kind != DeviceComputer || id.KindSource != KindFromThisComputer || id.KindConf != 100 {
		t.Errorf("it is this computer, known for certain: %s %d from %s", id.Kind, id.KindConf, id.KindSource)
	}
	if id.LANName != "" {
		t.Errorf("no inventory name is made up for it: %q", id.LANName)
	}
}

func TestTheHostCardIsMarkedWithoutTheNetworkInventory(t *testing.T) {
	r := devRig(t)
	rec, _, _ := r.s.recorder(r.s.h)
	rec.SetLAN(nil)
	rec.SetHost([]HostRadio{{Iface: "wlp2s0", MAC: neighbour}})
	idx := rec.LANIndex(context.Background())
	tag, err := rec.tag(context.Background(), tagDevice, neighbour[:])
	if err != nil {
		t.Fatal(err)
	}
	if l, ok := idx[string(tag)]; !ok || l.Self != "wlp2s0" {
		t.Fatalf("the host card is in the index with no inventory: %+v", idx)
	}
}

func TestAModuleMakerNamesNoKind(t *testing.T) {
	for _, maker := range []string{"Hui Zhou Gaoshengda", "GSD Viet Nam Technology"} {
		if g := GuessKind(KindInput{Maker: maker}); g.Kind != DeviceUnknown {
			t.Errorf("%s makes Wi-Fi modules for TVs, boxes and computers alike, so it names no kind: got %s", maker, g.Kind)
		}
	}
}

func TestYourOwnKindForTheHostCardOutranksThisComputer(t *testing.T) {
	r := devRig(t)
	rec, store, _ := r.s.recorder(r.s.h)
	confirmHomeAP(t, store, rec, false)
	rec.SetHost([]HostRadio{{Iface: "wlp2s0", MAC: neighbour}})
	for range 14 {
		cycleOf(t, rec, r.now, data(macAP), apBeacon(macAP, "home"), dataTo(macAP, neighbour))
	}
	if resp := callID(t, r, http.MethodPut, KindPath, shortOf(neighbour), `{"kind":"tv"}`); resp.Status != http.StatusOK {
		t.Fatalf("%d %s", resp.Status, resp.Body)
	}
	var v DeviceView
	if err := json.Unmarshal(callID(t, r, http.MethodGet, DeviceViewPath, shortOf(neighbour), "").Body, &v); err != nil {
		t.Fatal(err)
	}
	id := v.Device.Ident
	if id == nil || id.Kind != DeviceTV || id.KindSource != KindFromOwner || id.ThisComputer != "wlp2s0" || !id.LikelyYours() {
		t.Fatalf("your word for its kind stands, and it is still this computer: %+v", id)
	}
}

// The listening computer's own card is fixed to the sensor: its signal wobbles, but it never "gets closer" or "moves now".
func TestThisComputersOwnCardNeverMoves(t *testing.T) {
	r := devRig(t)
	rec, store, _ := r.s.recorder(r.s.h)
	confirmHomeAP(t, store, rec, false)
	rec.SetHost([]HostRadio{{Iface: "wlp2s0", MAC: neighbour}})
	for i := range 14 {
		dbm := -60
		if i%2 == 1 {
			dbm = -30 // a jump of 30 dB each cycle: movement for any other device
		}
		cycleOf(t, rec, r.now, data(macAP), apBeacon(macAP, "home"), dataAt(neighbour, dbm), dataTo(macAP, neighbour))
	}
	for _, d := range liveOf(t, r).Devices {
		if d.ID != shortOf(neighbour) {
			continue
		}
		if d.Ident == nil || d.Ident.ThisComputer == "" {
			t.Fatalf("the host card is known: %+v", d.Ident)
		}
		if d.Moving || d.Trend != TrendUnknown || d.Unusual {
			t.Errorf("this computer's own card never moves: moving %v trend %s unusual %v", d.Moving, d.Trend, d.Unusual)
		}
		return
	}
	t.Fatal("the host card is not in the picture")
}
