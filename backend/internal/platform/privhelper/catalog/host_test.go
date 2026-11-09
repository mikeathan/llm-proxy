package catalog

import (
	"errors"
	"slices"
	"testing"

	"llm-proxy/internal/platform/privhelper/wire"
)

func TestCaptureHostBuildsOnlyFixedArguments(t *testing.T) {
	p, err := prepare(t, "capture_host", `{"iface":"wlp2s0","seconds":30}`)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"capture-host", "--iface", "wlp2s0", "--seconds", "30"}; p.Program != "capture-host" || !slices.Equal(p.Args, want) {
		t.Errorf("got %s %v", p.Program, p.Args)
	}
	if p.Target != "" {
		t.Errorf("a capture touches no host, got target %q", p.Target)
	}
}

func TestCaptureHostDefaultsToTheLongestWindowInOneCanonicalForm(t *testing.T) {
	a, err := prepare(t, "capture_host", `{"iface":"eth0"}`)
	b, err2 := prepare(t, "capture_host", `{"iface":"eth0","seconds":60}`)
	if err != nil || err2 != nil || string(a.Canonical) != string(b.Canonical) {
		t.Fatalf("%s vs %s (%v, %v)", a.Canonical, b.Canonical, err, err2)
	}
}

func TestCaptureHostRefusesWhatItDoesNotUnderstand(t *testing.T) {
	cases := map[string]error{
		`{"iface":"eth0","seconds":61}`:                  ErrScope,
		`{"iface":"eth0","seconds":-1}`:                  ErrScope,
		`{"iface":"../x","seconds":5}`:                   ErrInvalid,
		`{"iface":"a b","seconds":5}`:                    ErrInvalid,
		`{"iface":"","seconds":5}`:                       ErrInvalid,
		`{"iface":"0123456789abcdef","seconds":5}`:       ErrInvalid,
		`{"iface":"eth0","seconds":5,"max_frames":9999}`: ErrInvalid,
		`{"iface":"eth0","seconds":5,"filter":"tcp"}`:    ErrInvalid,
		`{"iface":"eth0","seconds":5,"sample":1}`:        ErrInvalid,
		`{"seconds":5}`:                                  ErrInvalid,
		``:                                               ErrInvalid,
	}
	for params, want := range cases {
		if _, err := prepare(t, "capture_host", params); !errors.Is(err, want) {
			t.Errorf("%s: err = %v, want %v", params, err, want)
		}
	}
}

// The limits of Constitution I.13 are written in the catalogue's caps; this test is the place that fails when one is raised.
func TestCaptureHostCapsAreTheOnesTheConstitutionNames(t *testing.T) {
	a, err := Lookup("capture_host")
	if err != nil {
		t.Fatal(err)
	}
	if a.Jail.Network {
		t.Error("a capture sends nothing and needs no network in its jail")
	}
	if a.Class != ClassNet || a.Switch != SwitchKey {
		t.Errorf("class %q switch %q: it runs in the net unit and is switched on with the other actions", a.Class, a.Switch)
	}
	if a.Caps.MaxOutputBytes != 8<<20 || a.Caps.MaxPerHour != 12 || a.Caps.Timeout <= wire.HostMaxSeconds*1e9 {
		t.Errorf("caps = %+v, want 8 MiB, 12 an hour and time for the trailer", a.Caps)
	}
}

func TestCaptureHostIsOnlyInTheCatalogueOnce(t *testing.T) {
	n := 0
	for _, a := range All() {
		if a.ID == "capture_host" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d rows", n)
	}
}
