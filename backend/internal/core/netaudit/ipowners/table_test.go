package ipowners

import (
	"net/netip"
	"strings"
	"testing"
)

const sample = "1.0.0.0\t1.0.0.255\t13335\tUS\tCLOUDFLARENET - Cloudflare, Inc.\n" +
	"8.8.8.0\t8.8.8.255\t15169\tUS\tGOOGLE - Google LLC\n" +
	"2.17.116.0\t2.17.119.255\t8708\tRO\tDIGI-RO DIGI ROMANIA S.A.\n" +
	"5.0.0.0\t5.0.0.255\t0\tNone\tNot routed\n" +
	"2606:4700::\t2606:4700:ffff:ffff:ffff:ffff:ffff:ffff\t13335\tUS\tCLOUDFLARENET - Cloudflare, Inc.\n" +
	"2001:4860::\t2001:4860:ffff:ffff:ffff:ffff:ffff:ffff\t15169\tUS\tGOOGLE - Google LLC\n"

func mustParse(t *testing.T, text string) *Table {
	t.Helper()
	tab, err := Parse(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	return tab
}

func TestAnAddressInARangeHasItsOwnerAndCountry(t *testing.T) {
	tab := mustParse(t, sample)
	cases := []struct {
		addr    string
		asn     uint32
		country string
		name    string
	}{
		{"8.8.8.8", 15169, "US", "GOOGLE - Google LLC"},
		{"8.8.8.0", 15169, "US", "GOOGLE - Google LLC"},
		{"8.8.8.255", 15169, "US", "GOOGLE - Google LLC"},
		{"2.17.118.1", 8708, "RO", "DIGI-RO DIGI ROMANIA S.A."},
		{"1.0.0.1", 13335, "US", "CLOUDFLARENET - Cloudflare, Inc."},
		{"2606:4700::6810:84e5", 13335, "US", "CLOUDFLARENET - Cloudflare, Inc."},
		{"2001:4860:4860::8888", 15169, "US", "GOOGLE - Google LLC"},
		{"::ffff:8.8.8.8", 15169, "US", "GOOGLE - Google LLC"}, // an IPv4 address in its IPv6 form
	}
	for _, c := range cases {
		o, ok := tab.Lookup(netip.MustParseAddr(c.addr))
		if !ok || o.ASN != c.asn || o.Country != c.country || o.Name != c.name {
			t.Errorf("%s = %+v, %v; want AS%d %s %q", c.addr, o, ok, c.asn, c.country, c.name)
		}
	}
}

func TestAnAddressOutsideEveryRangeHasNoOwner(t *testing.T) {
	tab := mustParse(t, sample)
	for _, a := range []string{"8.8.7.255", "8.8.9.0", "0.0.0.1", "255.255.255.255", "5.0.0.7", "2606:4701::1", "::1", "10.0.0.1", "192.168.1.1"} {
		if o, ok := tab.Lookup(netip.MustParseAddr(a)); ok {
			t.Errorf("%s has the owner %+v, but it is in no range (or in the not-routed one)", a, o)
		}
	}
	if _, ok := (*Table)(nil).Lookup(netip.MustParseAddr("8.8.8.8")); ok {
		t.Error("a nil table found an owner")
	}
}

func TestARowThatIsNotWellFormedIsSkippedAndTheRestIsKept(t *testing.T) {
	bad := "x\ty\t1\tUS\tName\n" + // not addresses
		"9.9.9.9\t9.9.9.0\t1\tUS\tbackwards\n" + // end before start
		"9.9.9.0\t9.9.9.255\tabc\tUS\tnot a number\n" +
		"9.9.9.0\t9.9.9.255\n" + // too few columns
		"9.9.9.0\t2606::1\t1\tUS\tmixed families\n" +
		"# a comment\n\n" +
		"7.7.7.0\t7.7.7.255\t64496\tus\tlower-case country\n" +
		"6.6.6.0\t6.6.6.255\t64497\tUSA\tcountry too long\n"
	tab := mustParse(t, bad+sample)
	if o, ok := tab.Lookup(netip.MustParseAddr("8.8.8.8")); !ok || o.ASN != 15169 {
		t.Fatalf("the good rows after the bad ones were lost: %+v %v", o, ok)
	}
	for _, a := range []string{"9.9.9.9", "9.9.9.5"} {
		if o, ok := tab.Lookup(netip.MustParseAddr(a)); ok {
			t.Errorf("%s: a malformed row gave %+v", a, o)
		}
	}
	if o, ok := tab.Lookup(netip.MustParseAddr("7.7.7.7")); !ok || o.Country != "" {
		t.Errorf("a country that is not two capital letters must be dropped, the row kept: %+v %v", o, ok)
	}
	if o, _ := tab.Lookup(netip.MustParseAddr("6.6.6.6")); o.Country != "" {
		t.Errorf("a three-letter country was kept: %q", o.Country)
	}
}

func TestTheOwnerNameIsPlainBoundedText(t *testing.T) {
	long := strings.Repeat("a", 500)
	text := "9.9.9.0\t9.9.9.255\t1\tUS\tEvil \x1b[31m<script>alert(1)</script>‮ name\x00\n" +
		"9.9.8.0\t9.9.8.255\t2\tUS\t" + long + "\n"
	tab := mustParse(t, text)
	o, _ := tab.Lookup(netip.MustParseAddr("9.9.9.9"))
	for _, r := range o.Name {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) || r == 0x202e {
			t.Errorf("the name %q holds the control or direction character %U", o.Name, r)
		}
	}
	if strings.Contains(o.Name, "\x1b") || strings.Contains(o.Name, "\x00") {
		t.Errorf("name = %q", o.Name)
	}
	o2, _ := tab.Lookup(netip.MustParseAddr("9.9.8.8"))
	if len([]rune(o2.Name)) > maxNameRunes {
		t.Errorf("a name of %d runes was kept; the bound is %d", len([]rune(o2.Name)), maxNameRunes)
	}
}

func TestATableWithNoUsableRowIsAnErrorNotAnEmptyAnswer(t *testing.T) {
	for _, text := range []string{"", "# nothing\n", "garbage\n", "5.0.0.0\t5.0.0.255\t0\tNone\tNot routed\n"} {
		if _, err := Parse(strings.NewReader(text)); err == nil {
			t.Errorf("%q: an empty table was accepted; it would read as 'no one owns anything'", text)
		}
	}
}

func TestARangeThatOverlapsTheOneBeforeIsDroppedSoTheSearchStaysSound(t *testing.T) {
	text := "10.1.0.0\t10.1.0.255\t1\tUS\tfirst\n" +
		"10.1.0.128\t10.1.1.127\t2\tUS\toverlaps\n" +
		"10.1.2.0\t10.1.2.255\t3\tUS\tthird\n"
	tab := mustParse(t, text)
	if o, ok := tab.Lookup(netip.MustParseAddr("10.1.0.200")); !ok || o.ASN != 1 {
		t.Errorf("10.1.0.200 = %+v %v, want the first range", o, ok)
	}
	if o, ok := tab.Lookup(netip.MustParseAddr("10.1.2.9")); !ok || o.ASN != 3 {
		t.Errorf("10.1.2.9 = %+v %v, want the third range", o, ok)
	}
}

func TestTheTableIsBounded(t *testing.T) {
	was := maxRows
	maxRows = 100
	defer func() { maxRows = was }()
	var b strings.Builder
	for i := 0; i <= maxRows; i++ {
		a := netip.AddrFrom4([4]byte{byte(i >> 24), byte(i >> 16), byte(i >> 8), byte(i)})
		b.WriteString(a.String() + "\t" + a.String() + "\t1\tUS\tx\n")
	}
	if _, err := Parse(strings.NewReader(b.String())); err == nil {
		t.Error("a table of more than the allowed rows was accepted")
	}
}

func TestTheCountsSayWhatIsHeld(t *testing.T) {
	tab := mustParse(t, sample)
	if v4, v6 := tab.Counts(); v4 != 3 || v6 != 2 {
		t.Errorf("Counts = %d, %d; want 3, 2 (the not-routed row is not counted)", v4, v6)
	}
}
