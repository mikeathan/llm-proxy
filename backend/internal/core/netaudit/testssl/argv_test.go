package testssl

import (
	"net/netip"
	"slices"
	"testing"
)

func TestArgs_AreTheFixedListPlusTheTargetAndTheOutputFile(t *testing.T) {
	got, err := Args(Target{Addr: netip.MustParseAddr("192.168.50.10"), Port: 443}, "/scratch/out.json")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"--quiet", "--color", "0", "--warnings", "off",
		"--jsonfile", "/scratch/out.json", "--overwrite", "-p", "-s", "192.168.50.10:443",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got  %v\nwant %v", got, want)
	}
}

// The vulnerability checks send crafted packets and belong to the deep check only (ADR-0012); no flag of any other kind may appear.
func TestArgs_NeverAskForTheVulnerabilityChecksOrAnythingElse(t *testing.T) {
	got, err := Args(Target{Addr: netip.MustParseAddr("10.0.0.2"), Port: 8443}, "/s/o.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range got {
		switch a {
		case "-U", "--vulnerable", "-S", "--server-defaults", "-9", "--full", "--bugs", "--sneaky", "--mx", "--starttls", "--connect-timeout", "--openssl-timeout":
			t.Errorf("flag %q is not in the fixed list", a)
		}
	}
}

func TestArgs_BracketsAnIPv6AddressSoThePortIsNotReadAsPartOfIt(t *testing.T) {
	got, err := Args(Target{Addr: netip.MustParseAddr("fd00::1"), Port: 443}, "/s/o.json")
	if err != nil {
		t.Fatal(err)
	}
	if last := got[len(got)-1]; last != "[fd00::1]:443" {
		t.Errorf("target %q", last)
	}
}

func TestArgs_RefusesATargetThatIsNotAnAddressAndAPort(t *testing.T) {
	for _, tc := range []struct {
		name string
		tg   Target
		out  string
	}{
		{"no address", Target{Port: 443}, "/s/o.json"},
		{"port zero", Target{Addr: netip.MustParseAddr("10.0.0.2")}, "/s/o.json"},
		{"an address with a zone", Target{Addr: netip.MustParseAddr("fe80::1%eth0"), Port: 443}, "/s/o.json"},
		{"no output file", Target{Addr: netip.MustParseAddr("10.0.0.2"), Port: 443}, ""},
		{"an output name that would be read as a flag", Target{Addr: netip.MustParseAddr("10.0.0.2"), Port: 443}, "-oops.json"},
	} {
		if _, err := Args(tc.tg, tc.out); err == nil {
			t.Errorf("%s: want an error", tc.name)
		}
	}
}
