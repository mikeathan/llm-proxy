package radio

import (
	"strings"
	"testing"
)

const iwList = `Wiphy phy1
	max # scan SSIDs: 4
	Supported interface modes:
		 * IBSS
		 * managed
		 * AP
		 * monitor
		 * evil <script>
	Band 1:
		Capabilities: 0x1062
		HT Max RX data rate: 300
		Frequencies:
			* 2412 MHz [1] (20.0 dBm)
			* 2467 MHz [12] (disabled)
			* 2472 MHz [13] (20.0 dBm)
	Band 2:
		VHT Capabilities (0x339071b2):
		Frequencies:
			* 5180 MHz [36] (23.0 dBm)
			* 5300 MHz [60] (23.0 dBm) (radar detection)
	Supported extended features:
		* [ FTM_RESPONDER ]: ftm responder
Wiphy phy0
	Supported interface modes:
		 * managed
	Band 4:
		HE Iftypes: managed
		Frequencies:
			* 5955 MHz [1] (12.0 dBm)
`

func TestParseProbeReadsModesBandsAndGenerations(t *testing.T) {
	got := ParseProbe([]byte(iwList))
	if len(got) != 2 {
		t.Fatalf("%+v", got)
	}
	a := got[0]
	if a.Phy != "phy1" || !a.Monitor() || a.Channels24 != 2 || a.Channels5 != 2 || a.Channels6 != 0 || !a.HT || !a.VHT || a.HE || !a.Ranging {
		t.Errorf("%+v", a)
	}
	for _, m := range a.Modes {
		if strings.Contains(m, "evil") {
			t.Errorf("a mode that is not on the fixed list must be dropped: %v", a.Modes)
		}
	}
	if b := got[1]; b.Monitor() || !b.HE || b.Channels6 != 1 {
		t.Errorf("%+v", b)
	}
}

func TestParseProbeBoundsWhatItReads(t *testing.T) {
	if got := ParseProbe([]byte("not iw output\n\x00\xff")); len(got) != 0 {
		t.Errorf("%+v", got)
	}
	var many strings.Builder
	for range 40 {
		many.WriteString("Wiphy phy0\n")
	}
	if got := ParseProbe([]byte(many.String())); len(got) != maxPhys {
		t.Errorf("phys are capped: %d", len(got))
	}
	long := "Wiphy phy0\n" + strings.Repeat("x", 4*maxProbeLine) + "\n\t* 2412 MHz [1] (1 dBm)\n"
	_ = ParseProbe([]byte(long)) // an over-long line must not panic or hang
}

func FuzzParseProbe(f *testing.F) {
	f.Add([]byte(iwList))
	f.Fuzz(func(t *testing.T, b []byte) {
		for _, p := range ParseProbe(b) {
			if p.Channels24 < 0 || p.Channels5 < 0 || p.Channels6 < 0 {
				t.Fatal("negative count")
			}
		}
	})
}
