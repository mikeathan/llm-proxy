package oui

import (
	"bytes"
	"compress/gzip"
	"io"
	"strings"
	"testing"
)

// Rows copied from the IEEE registry file (https://standards-oui.ieee.org/oui/oui.csv, fetched 2026-11-02): a quoted name
// with a comma, a name with a trailing space in the address column, and an entry whose owner asked not to be named.
const sample = `Registry,Assignment,Organization Name,Organization Address
MA-L,E80AB9,"Cisco Systems, Inc",80 West Tasman Drive San Jose CA US 94568 
MA-L,9075DE,Zebra Technologies Inc.,ONE ZEBRA PLAZA HOLTSVILLE NY US 11742 
MA-L,0050C2,Private,
MA-M,70B3D5A,Some Small Maker,Elsewhere
`

func TestConvert_KeepsNamedThreeByteAssignmentsOnly(t *testing.T) {
	var out bytes.Buffer
	if err := Convert(strings.NewReader(sample), &out); err != nil {
		t.Fatal(err)
	}
	zr, err := gzip.NewReader(&out)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(zr)
	want := "9075DE\tZebra Technologies Inc.\nE80AB9\tCisco Systems, Inc\n"
	if string(raw) != want {
		t.Errorf("got %q want %q", raw, want)
	}
}

func TestParse_LookupByAnyMACSpelling(t *testing.T) {
	tab, err := parse(strings.NewReader("9075DE\tZebra Technologies Inc.\nE80AB9\tCisco Systems, Inc\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, mac := range []string{"e8:0a:b9:12:34:56", "E8:0A:B9:12:34:56", "e8-0a-b9-12-34-56"} {
		if got := tab.vendor(mac); got != "Cisco Systems, Inc" {
			t.Errorf("%s: got %q", mac, got)
		}
	}
}

func TestVendor_NothingForRandomizedUnknownOrMalformedAddresses(t *testing.T) {
	tab, _ := parse(strings.NewReader("9075DE\tZebra Technologies Inc.\n"))
	// 0x92 has the locally administered bit set: a phone hiding its hardware address, so a prefix match would be wrong.
	for _, mac := range []string{"92:75:de:00:00:01", "aa:bb:cc:00:00:01", "", "not-a-mac", "90:75:de"} {
		if got := tab.vendor(mac); got != "" {
			t.Errorf("%q: got %q, want none", mac, got)
		}
	}
}

func TestVendor_BundledRegistryKnowsRealPrefixes(t *testing.T) {
	// Both prefixes come from the registry rows above; the bundled file must hold at least them.
	if got := Vendor("e8:0a:b9:00:00:01"); got != "Cisco Systems, Inc" {
		t.Errorf("Cisco: got %q", got)
	}
	if got := Vendor("90:75:de:00:00:01"); !strings.HasPrefix(got, "Zebra") {
		t.Errorf("Zebra: got %q", got)
	}
}
