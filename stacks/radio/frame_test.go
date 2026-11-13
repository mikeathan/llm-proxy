package radio

import (
	"encoding/binary"
	"errors"
	"testing"
)

// Synthetic frames only: every address below is made up (locally administered 02:..), no real capture is committed (plan radio-stack R2).
var (
	macAP     = MAC{0x02, 0, 0, 0, 0, 0x01}
	macPhone  = MAC{0x02, 0, 0, 0, 0, 0x02}
	macBroad  = MAC{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	rtapSig60 = []byte{ // radiotap: flags, rate 6 Mb/s, channel 5300 MHz, signal -60 dBm
		0, 0, 0, 0, // header, length patched in rtap()
		0x2e, 0, 0, 0, // present: flags, rate, channel, antsignal (bits 1,2,3,5)
		0x00,       // flags
		0x0c,       // rate 12 * 500 kb/s
		0xb4, 0x14, // 5300 MHz
		0x40, 0x01, // channel flags
		0xc4, // -60 dBm
	}
)

func rtap(body []byte) []byte {
	h := append([]byte{}, rtapSig60...)
	binary.LittleEndian.PutUint16(h[2:], uint16(len(h)))
	return append(h, body...)
}

func hdr(fc0, fc1 byte, a1, a2, a3 MAC) []byte {
	b := []byte{fc0, fc1, 0, 0}
	b = append(b, a1[:]...)
	b = append(b, a2[:]...)
	b = append(b, a3[:]...)
	return append(b, 0, 0) // sequence control
}

func ie(id byte, data ...byte) []byte { return append([]byte{id, byte(len(data))}, data...) }

func cat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

var rsnWPA2 = ie(48, 1, 0, 0x00, 0x0f, 0xac, 4, 1, 0, 0x00, 0x0f, 0xac, 4, 1, 0, 0x00, 0x0f, 0xac, 2, 0x00, 0x00)
var rsnWPA3 = ie(48, 1, 0, 0x00, 0x0f, 0xac, 4, 1, 0, 0x00, 0x0f, 0xac, 4, 1, 0, 0x00, 0x0f, 0xac, 8, 0xc0, 0x00)

func beacon(ssid string, ies ...[]byte) []byte {
	fixed := make([]byte, 12)
	binary.LittleEndian.PutUint16(fixed[8:], 100) // interval
	binary.LittleEndian.PutUint16(fixed[10:], 0x0401)
	return rtap(cat(hdr(0x80, 0, macBroad, macAP, macAP), fixed, ie(0, []byte(ssid)...), ie(3, 60), cat(ies...)))
}

func TestParseBeaconSecurity(t *testing.T) {
	cases := []struct {
		name string
		ies  [][]byte
		want Security
	}{
		{"wpa2", [][]byte{rsnWPA2}, Security{Mode: SecWPA2, PMFCapable: false}},
		{"wpa3", [][]byte{rsnWPA3}, Security{Mode: SecWPA3, PMFRequired: true, PMFCapable: true}},
	}
	for _, c := range cases {
		f, err := Parse(beacon("home", c.ies...))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if f.Kind != KindBeacon || f.SSID != "home" || f.Channel != 60 || f.BSSID != macAP {
			t.Fatalf("%s: got %+v", c.name, f)
		}
		if f.Security != c.want {
			t.Errorf("%s: security %+v want %+v", c.name, f.Security, c.want)
		}
		if f.Beacon.IntervalTU != 100 {
			t.Errorf("%s: interval %d", c.name, f.Beacon.IntervalTU)
		}
	}
}

func TestParseRadiotapFacts(t *testing.T) {
	f, err := Parse(beacon("x", rsnWPA2))
	if err != nil {
		t.Fatal(err)
	}
	if !f.SignalOK || f.Signal != -60 || f.FreqMHz != 5300 || f.RateKbps != 6000 {
		t.Errorf("radiotap facts: %+v", f)
	}
}

func TestParseOpenAndWEP(t *testing.T) {
	f, _ := Parse(beacon("cafe"))
	if f.Security.Mode != SecOpen {
		t.Errorf("open: %+v", f.Security)
	}
	// Privacy bit set, no RSN or WPA element: WEP.
	b := beacon("old")
	off := len(rtapSig60) + 24 + 10
	b[off] |= 0x10
	if f, _ = Parse(b); f.Security.Mode != SecWEP {
		t.Errorf("wep: %+v", f.Security)
	}
}

func TestParseHiddenSSID(t *testing.T) {
	f, _ := Parse(beacon(""))
	if !f.HiddenSSID || f.SSID != "" {
		t.Errorf("empty: %+v", f)
	}
	f, _ = Parse(beacon("\x00\x00\x00"))
	if !f.HiddenSSID {
		t.Errorf("zeroed: %+v", f)
	}
}

func TestParseSSIDIsSanitised(t *testing.T) {
	f, _ := Parse(beacon("a\x1b[2Jb\xff"))
	for _, r := range f.SSID {
		if r < 0x20 || r == 0x7f {
			t.Errorf("control character kept in %q", f.SSID)
		}
	}
}

func TestParseProbeRequest(t *testing.T) {
	b := rtap(cat(hdr(0x40, 0, macBroad, macPhone, macBroad), ie(0, []byte("home")...)))
	f, err := Parse(b)
	if err != nil || f.Kind != KindProbeRequest || f.SSID != "home" || f.Transmitter != macPhone {
		t.Fatalf("%+v %v", f, err)
	}
	// A wildcard probe has an empty name.
	f, _ = Parse(rtap(cat(hdr(0x40, 0, macBroad, macPhone, macBroad), ie(0))))
	if f.SSID != "" || f.Kind != KindProbeRequest {
		t.Errorf("wildcard: %+v", f)
	}
}

func TestParseAuthDeauthDisassoc(t *testing.T) {
	auth := rtap(cat(hdr(0xb0, 0, macAP, macPhone, macAP), []byte{0, 0, 1, 0, 0, 0}))
	f, err := Parse(auth)
	if err != nil || f.Kind != KindAuthentication || f.Auth.Algorithm != 0 || f.Auth.Sequence != 1 || f.Auth.Status != 0 {
		t.Fatalf("auth: %+v %v", f, err)
	}
	deauth := rtap(cat(hdr(0xc0, 0, macPhone, macAP, macAP), []byte{3, 0}))
	if f, _ = Parse(deauth); f.Kind != KindDeauthentication || f.Reason != 3 {
		t.Errorf("deauth: %+v", f)
	}
	dis := rtap(cat(hdr(0xa0, 0, macPhone, macAP, macAP), []byte{8, 0}))
	if f, _ = Parse(dis); f.Kind != KindDisassociation || f.Reason != 8 {
		t.Errorf("disassoc: %+v", f)
	}
}

func TestParseAssociationRequest(t *testing.T) {
	b := rtap(cat(hdr(0x00, 0, macAP, macPhone, macAP), []byte{0x11, 0x04, 10, 0}, ie(0, []byte("home")...)))
	f, err := Parse(b)
	if err != nil || f.Kind != KindAssociationRequest || f.SSID != "home" || f.BSSID != macAP {
		t.Fatalf("%+v %v", f, err)
	}
}

func TestParseControlAndData(t *testing.T) {
	cts := rtap([]byte{0xc4, 0, 0, 0, 2, 0, 0, 0, 0, 1})
	f, err := Parse(cts)
	if err != nil || f.Kind != KindControl || f.Receiver != macAP {
		t.Fatalf("cts: %+v %v", f, err)
	}
	rts := rtap(cat([]byte{0xb4, 0, 0, 0}, macAP[:], macPhone[:]))
	if f, err = Parse(rts); err != nil || f.Kind != KindControl || f.Transmitter != macPhone {
		t.Fatalf("rts: %+v %v", f, err)
	}
	// QoS data, to the AP (ToDS), retry flag.
	data := rtap(cat(hdr(0x88, 0x09, macAP, macPhone, macBroad), []byte{0, 0}, make([]byte, 40)))
	f, err = Parse(data)
	if err != nil || f.Kind != KindData || !f.Retry || f.Transmitter != macPhone || f.BSSID != macAP {
		t.Fatalf("data: %+v %v", f, err)
	}
	if f.PayloadLen != 40 {
		t.Errorf("payload %d", f.PayloadLen)
	}
}

func TestParseRejectsBadInput(t *testing.T) {
	good := beacon("home", rsnWPA2)
	for n := 0; n < len(good); n++ {
		// No prefix may panic; short ones must fail with ErrTruncated or parse as a shorter frame.
		_, _ = Parse(good[:n])
	}
	if _, err := Parse(nil); !errors.Is(err, ErrTruncated) {
		t.Errorf("nil: %v", err)
	}
	bad := append([]byte{}, good...)
	binary.LittleEndian.PutUint16(bad[2:], 0xffff) // radiotap length past the end
	if _, err := Parse(bad); !errors.Is(err, ErrTruncated) {
		t.Errorf("long radiotap: %v", err)
	}
	bad = append([]byte{}, good...)
	bad[0] = 3 // radiotap version
	if _, err := Parse(bad); !errors.Is(err, ErrNotRadiotap) {
		t.Errorf("version: %v", err)
	}
	bad = append([]byte{}, good...)
	bad[len(rtapSig60)] |= 0x01 // 802.11 protocol version 1
	if _, err := Parse(bad); !errors.Is(err, ErrNotWiFi) {
		t.Errorf("protocol: %v", err)
	}
}

func TestParseLyingInformationElementLength(t *testing.T) {
	b := beacon("home", []byte{221, 200, 1, 2})
	f, err := Parse(b)
	if err != nil || f.SSID != "home" {
		t.Errorf("a lying element must not break the frame: %+v %v", f, err)
	}
}

func TestParseFCSStripped(t *testing.T) {
	b := beacon("home", rsnWPA2)
	b[len(rtapSig60)-7] = 0x10 // flags: FCS at the end
	b = append(b, 1, 2, 3, 4)
	f, err := Parse(b)
	if err != nil || f.Kind != KindBeacon || f.Security.Mode != SecWPA2 {
		t.Errorf("%+v %v", f, err)
	}
}

func TestParseMultipleAntennas(t *testing.T) {
	// Present words: word 0 carries flags, antsignal, antenna and the "radiotap namespace next" and "ext" bits; word 1 repeats antsignal and antenna.
	h := []byte{
		0, 0, 0, 0,
		0x20, 0x08, 0x00, 0xa0, // bits 5 (antsignal), 11 (antenna), 29 (next word is radiotap), 31 (more words)
		0x20, 0x08, 0x00, 0x00, // second word: 5, 11
		0xc8, 0, // -56 dBm, antenna 0
		0xc4, 1, // -60 dBm, antenna 1
	}
	binary.LittleEndian.PutUint16(h[2:], uint16(len(h)))
	// pad to the header: signal(1) antenna(1) signal(1) antenna(1)
	frame := append(h, hdr(0x80, 0, macBroad, macAP, macAP)...)
	frame = append(frame, make([]byte, 12)...)
	f, err := Parse(frame)
	if err != nil || !f.SignalOK || f.Signal != -56 {
		t.Errorf("want the strongest antenna (-56), got %+v %v", f, err)
	}
}

func FuzzParse(f *testing.F) {
	f.Add(beacon("home", rsnWPA2))
	f.Add(beacon("x", rsnWPA3))
	f.Add(rtap(cat(hdr(0x40, 0, macBroad, macPhone, macBroad), ie(0, []byte("home")...))))
	f.Add(rtap(cat(hdr(0xc0, 0, macPhone, macAP, macAP), []byte{3, 0})))
	f.Add(rtap(cat(hdr(0x88, 0x09, macAP, macPhone, macBroad), []byte{0, 0}, make([]byte, 40))))
	f.Fuzz(func(t *testing.T, b []byte) {
		fr, err := Parse(b)
		if err == nil && len(fr.SSID) > 32*3 {
			t.Fatalf("SSID longer than allowed: %d", len(fr.SSID))
		}
	})
}
