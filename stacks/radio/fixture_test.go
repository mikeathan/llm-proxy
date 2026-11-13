package radio

import (
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

// The fixtures in testdata are real frames from the owner's capture of 2026-11-13 (radiotap header untouched) with every address replaced by
// 02:00:00:00:00:NN, every network name by x characters and every element but name, channel, HT, RSN, VHT, HE and HT operation removed. The
// expected values are what `tcpdump -nn -e` printed for the original frames.
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name + ".hex")
	if err != nil {
		t.Fatal(err)
	}
	out, err := hex.DecodeString(strings.TrimSpace(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRealBeaconFixture(t *testing.T) {
	f, err := Parse(fixture(t, "beacon"))
	if err != nil {
		t.Fatal(err)
	}
	if f.Kind != KindBeacon || f.Channel != 60 || f.FreqMHz != 5300 || f.Security.Mode != SecWPA2 || f.Beacon.Generation != 6 {
		t.Errorf("%+v", f)
	}
	if strings.Trim(f.SSID, "x") != "" || f.SSID == "" {
		t.Errorf("name should be the anonymised x run, got %q", f.SSID)
	}
}

func TestRealProbeRequestFixture(t *testing.T) {
	f, err := Parse(fixture(t, "probe"))
	if err != nil || f.Kind != KindProbeRequest || !f.HasTransmitter || !f.SignalOK {
		t.Fatalf("%+v %v", f, err)
	}
}

func TestRealDeauthFixture(t *testing.T) {
	f, err := Parse(fixture(t, "deauth"))
	if err != nil || f.Kind != KindDeauthentication {
		t.Fatalf("%+v %v", f, err)
	}
	// tcpdump: "-34dBm signal -35dBm signal antenna 0 -34dBm signal antenna 1 ... leaving (or has left) IBSS or ESS" = reason 3, 6.0 Mb/s, 5300 MHz.
	if f.Reason != 3 || f.Signal != -34 || f.RateKbps != 6000 || f.FreqMHz != 5300 {
		t.Errorf("%+v", f)
	}
}
