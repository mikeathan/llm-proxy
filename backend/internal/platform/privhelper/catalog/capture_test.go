package catalog

import (
	"errors"
	"slices"
	"testing"

	"llm-proxy/internal/platform/privhelper/wire"
)

func TestCaptureWindowBuildsOnlyFixedArguments(t *testing.T) {
	p, err := prepare(t, "capture_window", `{"iface":"wlp2s0","seconds":30}`)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"capture", "--iface", "wlp2s0", "--seconds", "30"}; p.Program != "capture" || !slices.Equal(p.Args, want) {
		t.Errorf("got %s %v", p.Program, p.Args)
	}
	if p.Target != "" {
		t.Errorf("a capture touches no host, got target %q", p.Target)
	}
}

func TestCaptureWindowDefaultsToTheLongestWindowInOneCanonicalForm(t *testing.T) {
	a, err := prepare(t, "capture_window", `{"iface":"eth0"}`)
	b, err2 := prepare(t, "capture_window", `{"iface":"eth0","seconds":60}`)
	if err != nil || err2 != nil || string(a.Canonical) != string(b.Canonical) {
		t.Fatalf("%s vs %s (%v, %v)", a.Canonical, b.Canonical, err, err2)
	}
}

func TestCaptureWindowRefusesWhatItDoesNotUnderstand(t *testing.T) {
	cases := map[string]error{
		`{"iface":"eth0","seconds":61}`:                  ErrScope,
		`{"iface":"eth0","seconds":-1}`:                  ErrScope,
		`{"iface":"../x","seconds":5}`:                   ErrInvalid,
		`{"iface":"a b","seconds":5}`:                    ErrInvalid,
		`{"iface":"","seconds":5}`:                       ErrInvalid,
		`{"iface":"0123456789abcdef","seconds":5}`:       ErrInvalid,
		`{"iface":"eth0","seconds":5,"max_frames":9999}`: ErrInvalid,
		`{"iface":"eth0","seconds":5,"filter":"tcp"}`:    ErrInvalid,
		`{"seconds":5}`:                                  ErrInvalid,
		``:                                               ErrInvalid,
	}
	for params, want := range cases {
		if _, err := prepare(t, "capture_window", params); !errors.Is(err, want) {
			t.Errorf("%s: err = %v, want %v", params, err, want)
		}
	}
}

func TestCaptureWindowCapsFitTheWireAndLeaveTimeForTheTrailer(t *testing.T) {
	a, err := Lookup("capture_window")
	if err != nil {
		t.Fatal(err)
	}
	if a.Jail.Network {
		t.Error("a capture sends nothing and needs no network in its jail")
	}
	if a.Caps.MaxOutputBytes != wire.CaptureMaxOutput || a.Caps.Timeout <= wire.CaptureMaxSeconds*1e9 || a.Caps.MaxPerHour > 12 {
		t.Errorf("caps = %+v", a.Caps)
	}
}
