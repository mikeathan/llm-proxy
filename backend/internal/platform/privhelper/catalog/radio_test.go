package catalog

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"llm-proxy/internal/platform/privhelper/wire"
)

func TestRadioActionsAreInTheRadioClassAndTheRestAreNot(t *testing.T) {
	for _, a := range All() {
		isRadio := a.ID == "radio_probe" || a.ID == "radio_listen"
		if (a.Class == ClassRadio) != isRadio {
			t.Errorf("%s is in class %s", a.ID, a.Class)
		}
	}
}

func TestRadioProbeTakesNoParametersAndReadsOnly(t *testing.T) {
	p, err := prepare(t, "radio_probe", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if p.Program != "iw" || !slices.Equal(p.Args, []string{"list"}) || len(p.Before) != 0 || len(p.After) != 0 || p.Iface != "" {
		t.Errorf("%+v", p)
	}
	if _, err := prepare(t, "radio_probe", `{"iface":"wlan0"}`); !errors.Is(err, ErrInvalid) {
		t.Errorf("a probe names nothing: %v", err)
	}
}

func TestRadioListenBuildsTheWholeSequenceFromFixedWords(t *testing.T) {
	p, err := prepare(t, "radio_listen", `{"iface":"wlxf0090da058a3","channel":60,"seconds":20}`)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"radio-listen", "--iface", "wlxf0090da058a3", "--seconds", "20"}; p.Program != "radio-listen" || !slices.Equal(p.Args, want) {
		t.Errorf("got %s %v", p.Program, p.Args)
	}
	if p.Iface != "wlxf0090da058a3" {
		t.Errorf("iface %q", p.Iface)
	}
	var before, after []string
	for _, s := range p.Before {
		before = append(before, s.Program+" "+strings.Join(s.Args, " "))
	}
	for _, s := range p.After {
		after = append(after, s.Program+" "+strings.Join(s.Args, " "))
	}
	wantBefore := []string{
		"ip link set dev wlxf0090da058a3 down", "iw dev wlxf0090da058a3 set type monitor", "ip link set dev wlxf0090da058a3 up",
		"iw dev wlxf0090da058a3 set channel 60", "iw dev wlxf0090da058a3 info",
	}
	wantAfter := []string{"ip link set dev wlxf0090da058a3 down", "iw dev wlxf0090da058a3 set type managed", "ip link set dev wlxf0090da058a3 up"}
	if !slices.Equal(before, wantBefore) || !slices.Equal(after, wantAfter) {
		t.Errorf("before %q\nafter %q", before, after)
	}
	if !p.Before[len(p.Before)-1].Report || p.Before[0].Report {
		t.Error("only the channel read-back is reported")
	}
	for _, s := range append(slices.Clone(p.Before), p.After...) {
		for _, arg := range s.Args {
			if strings.ContainsAny(arg, " ;|&$`\n") {
				t.Errorf("an argument is not one plain word: %q", arg)
			}
		}
	}
}

func TestRadioListenDefaultsTheWindowInOneCanonicalForm(t *testing.T) {
	a, err := prepare(t, "radio_listen", `{"iface":"wlan1","channel":1}`)
	b, err2 := prepare(t, "radio_listen", `{"iface":"wlan1","channel":1,"seconds":20}`)
	if err != nil || err2 != nil || string(a.Canonical) != string(b.Canonical) {
		t.Fatalf("%s vs %s (%v, %v)", a.Canonical, b.Canonical, err, err2)
	}
}

func TestRadioListenRefusesWhatItDoesNotUnderstand(t *testing.T) {
	cases := map[string]error{
		`{"iface":"wlan1","channel":14,"seconds":5}`:                 ErrScope, // not in the list
		`{"iface":"wlan1","channel":0,"seconds":5}`:                  ErrScope,
		`{"iface":"wlan1","channel":1,"seconds":31}`:                 ErrScope,
		`{"iface":"wlan1","channel":1,"seconds":-1}`:                 ErrScope,
		`{"iface":"../x","channel":1,"seconds":5}`:                   ErrInvalid,
		`{"iface":"a b","channel":1,"seconds":5}`:                    ErrInvalid,
		`{"iface":"","channel":1,"seconds":5}`:                       ErrInvalid,
		`{"iface":"0123456789abcdef","channel":1,"seconds":5}`:       ErrInvalid,
		`{"iface":"wlan1","channel":1,"seconds":5,"type":"managed"}`: ErrInvalid,
		`{"iface":"wlan1","channel":1,"seconds":5,"transmit":true}`:  ErrInvalid,
		`{"channel":1,"seconds":5}`:                                  ErrScope, // no iface: empty fails the name check below
		`{"iface":"wlan1","channel":"1","seconds":5}`:                ErrInvalid,
		``: ErrInvalid,
	}
	for params, want := range cases {
		_, err := prepare(t, "radio_listen", params)
		if want == ErrScope && !strings.Contains(params, "iface") {
			want = ErrInvalid
		}
		if !errors.Is(err, want) {
			t.Errorf("%s: err = %v, want %v", params, err, want)
		}
	}
}

func TestRadioListenCapsFitTheWireAndLeaveTimeToPutTheAdapterBack(t *testing.T) {
	a, err := Lookup("radio_listen")
	if err != nil {
		t.Fatal(err)
	}
	if a.Jail.Network || a.Caps.MaxOutputBytes != wire.RadioMaxOutput || a.Caps.Timeout < (wire.RadioMaxSeconds+20)*1e9 {
		t.Errorf("caps = %+v jail = %+v", a.Caps, a.Jail)
	}
}

func TestEveryRadioChannelIsAKnownWifiChannel(t *testing.T) {
	for _, c := range radioChannels {
		if c < 1 || c > 165 || (c > 13 && c < 36) {
			t.Errorf("channel %d is not a 2.4 or 5 GHz channel", c)
		}
	}
}

// Only the two radio actions may run under a bounded session; anything that scans, sends or captures this computer's own traffic needs a person's
// step-up for every request (Constitution I.17). A new action is not standing until someone writes it in code and this list.
func TestOnlyTheRadioActionsMayRunUnderASession(t *testing.T) {
	var got []string
	for _, a := range All() {
		if a.Standing {
			got = append(got, a.ID)
		}
	}
	if want := []string{"radio_probe", "radio_listen"}; !slices.Equal(got, want) {
		t.Errorf("standing actions = %v, want %v", got, want)
	}
}
