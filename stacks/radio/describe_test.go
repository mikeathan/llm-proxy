package radio

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"testing/fstest"
	"time"

	"llm-proxy-stacks/sdk"
)

func TestDescribeTellsTheTruthAboutWhatTheAdapterCanDo(t *testing.T) {
	sys := fstest.MapFS{}
	usbWifi(sys, "wlx00", "rtw88_8822bu", "2357", "012e", "803", "down")
	pciWifi(sys, "wlp2s0", "up")
	read := &ProbeRead{At: t0, Phys: ParseProbe([]byte(iwList))}
	phy := func(n string) string {
		if n == "wlx00" {
			return "phy1"
		}
		return "phy0"
	}
	got := Describe(Discover(sys).Sensors, read, "wlx00", phy)

	usb := byID(t, got, "wlx00")
	if usb.Role != RoleListening || usb.Probe == nil || !usb.Probe.Monitor {
		t.Fatalf("an adapter that is listening says so: %+v", usb)
	}
	for id, want := range map[string]Use{"monitor_capture": UseUsed, "he_decode": UseUnsupported, "six_ghz": UseUnsupported, "ranging": UseNotBuilt, "frame_injection": UseByDesign} {
		if c := capability(t, usb, id); c.Use != want || !c.Known {
			t.Errorf("%s: %+v, want %s and read from the adapter", id, c, want)
		}
	}
	if usb.Probe.Bands[0] != "2.4 GHz" || usb.Probe.Standards[len(usb.Probe.Standards)-1] != "Wi-Fi 5" {
		t.Errorf("%+v", usb.Probe)
	}

	card := byID(t, got, "wlp2s0")
	if card.Role != RoleWontUse || len(card.Capabilities) != 2 {
		t.Errorf("the built-in card is one line and a reason, not six rows: %+v", card)
	}

	idle := Describe(Discover(sys).Sensors, read, "", phy)
	if r := byID(t, idle, "wlx00"); r.Role != RoleCanListen || capability(t, r, "monitor_capture").Use != UseReady {
		t.Errorf("an adapter not listening is ready, not 'not built': %+v", r)
	}
	if unusedCount(got) != 1 {
		t.Errorf("only what could still be built counts as unused: %d", unusedCount(got))
	}
}

func TestTheRunReadsTheProbeAndTheSensorsPageShowsIt(t *testing.T) {
	r := newRig(t, twoChannels, okWindow)
	sys := fstest.MapFS{}
	usbWifi(sys, "wlx00", "rtw88_8822bu", "2357", "012e", "803", "down")
	sys["devices/pci0000:00/0000:00:14.0/usb1/1-2/1-2:1.0/net/wlx00/phy80211/name"] = &fstest.MapFile{Data: []byte("phy1\n")}
	r.s.sys = sys

	r.post(t, `{"adapter":"wlx00","hours":1}`)
	waitFor(t, "the first cycle", func() bool { return r.priv.count() == 2 })
	r.priv.mu.Lock()
	probes := r.priv.probes
	r.priv.mu.Unlock()
	if probes != 1 {
		t.Fatalf("one probe at the start of a run, asked %d", probes)
	}
	resp, err := r.s.Handle(context.Background(), stacksdk.Request{Method: http.MethodGet, Path: RoutePath})
	if err != nil || resp.Status != http.StatusOK {
		t.Fatalf("%+v %v", resp, err)
	}
	var v View
	if err := json.Unmarshal(resp.Body, &v); err != nil {
		t.Fatal(err)
	}
	s := byID(t, v.Sensors, "wlx00")
	if s.Role != RoleListening || s.Probe == nil || s.Probe.ReadAt != t0.Format(time.DateOnly) || !s.Probe.Monitor {
		t.Errorf("%+v", s)
	}
}
