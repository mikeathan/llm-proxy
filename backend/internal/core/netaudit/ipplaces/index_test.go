package ipplaces

import (
	"bytes"
	"net/netip"
	"strings"
	"testing"
)

// memFile is an io.WriterAt and io.ReaderAt over a byte slice.
type memFile struct{ b []byte }

func (m *memFile) WriteAt(p []byte, off int64) (int, error) {
	if end := int(off) + len(p); end > len(m.b) {
		m.b = append(m.b, make([]byte, end-len(m.b))...)
	}
	copy(m.b[off:], p)
	return len(p), nil
}
func (m *memFile) ReadAt(p []byte, off int64) (int, error) { return copy(p, m.b[off:]), nil }

const sample = `0.0.0.0,0.255.255.255,ZZ,ZZ,,,0,0
1.0.0.0,1.0.0.255,OC,AU,Queensland,"South Brisbane",-27.4767,153.017
1.0.1.0,1.0.1.255,OC,AU,Queensland,"South Brisbane",-27.4767,153.017
1.0.2.0,1.0.3.255,AS,CN,Fujian,Wenquan,26.0998,119.297
1.0.9.0,1.0.9.255,AS,CN,,,0,0
1.1.0.0,1.1.0.255,EU,IE,Leinster,Dublin,53.3498,-6.26031
1.1.0.200,1.1.0.250,EU,IE,Leinster,Overlap,1,1
2001:db8::,2001:db8:ffff:ffff:ffff:ffff:ffff:ffff,EU,DE,Hesse,Frankfurt,50.1,8.6
2001:db9::,2001:db9::ffff,EU,DE,"Hesse <b>x</b>",Frankfurt,50.1,8.6
`

func build(t *testing.T, csv string) *Index {
	t.Helper()
	f := &memFile{}
	if _, err := Build(strings.NewReader(csv), f); err != nil {
		t.Fatal(err)
	}
	x, err := Open(bytes.NewReader(f.b), int64(len(f.b)))
	if err != nil {
		t.Fatal(err)
	}
	return x
}

func at(t *testing.T, x *Index, addr string) (Place, bool) {
	t.Helper()
	p, ok, err := x.Lookup(netip.MustParseAddr(addr))
	if err != nil {
		t.Fatal(err)
	}
	return p, ok
}

func TestLookupFindsTheCityAndRegionOfAnAddress(t *testing.T) {
	x := build(t, sample)
	for addr, want := range map[string]Place{
		"1.0.0.0":     {Region: "Queensland", City: "South Brisbane"},
		"1.0.1.255":   {Region: "Queensland", City: "South Brisbane"}, // two neighbouring rows of one place are one run
		"1.0.3.1":     {Region: "Fujian", City: "Wenquan"},
		"1.1.0.255":   {Region: "Leinster", City: "Dublin"},
		"2001:db8::1": {Region: "Hesse", City: "Frankfurt"},
	} {
		got, ok := at(t, x, addr)
		if !ok || got != want {
			t.Errorf("%s = %+v %v, want %+v", addr, got, ok, want)
		}
	}
}

func TestAnAddressInAGapOrWithNoCityHasNoPlace(t *testing.T) {
	x := build(t, sample)
	for _, addr := range []string{"0.0.0.1", "1.0.4.0", "1.0.9.5", "1.2.0.0", "200.1.1.1", "2001:dba::1", "::1"} {
		if p, ok := at(t, x, addr); ok {
			t.Errorf("%s has place %+v, want none", addr, p)
		}
	}
}

func TestARowInsideThePreviousOneIsDroppedAndNamesAreMadePlain(t *testing.T) {
	x := build(t, sample)
	if p, _ := at(t, x, "1.1.0.220"); p.City != "Dublin" {
		t.Errorf("an overlapping row replaced the earlier one: %+v", p)
	}
	if p, _ := at(t, x, "2001:db9::1"); strings.ContainsAny(p.Region, "\x00") || p.City != "Frankfurt" {
		t.Errorf("got %+v", p)
	}
}

func TestBuildCountsRunsAndPlacesAndRefusesAnEmptyOrDisorderedTable(t *testing.T) {
	f := &memFile{}
	c, err := Build(strings.NewReader(sample), f)
	if err != nil || c.Places != 5 || c.V4 == 0 || c.V6 == 0 || c.Ranges() != c.V4+c.V6 {
		t.Fatalf("counts %+v err %v", c, err)
	}
	if _, err := Build(strings.NewReader("junk\n0.0.0.0,0.255.255.255,ZZ,ZZ,,,0,0\n"), &memFile{}); err == nil {
		t.Error("a table with no usable range must be an error")
	}
	if _, err := Build(strings.NewReader("2001:db8::,2001:db8::ff,EU,DE,Hesse,Frankfurt,1,1\n1.0.0.0,1.0.0.255,OC,AU,Q,B,1,1\n"), &memFile{}); err == nil {
		t.Error("an IPv4 row after an IPv6 row must be an error")
	}
}

func TestOpenRefusesAFileThatIsNotAnIndexOrIsCutShort(t *testing.T) {
	f := &memFile{}
	if _, err := Build(strings.NewReader(sample), f); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(bytes.NewReader(f.b[:len(f.b)-3]), int64(len(f.b)-3)); err == nil {
		t.Error("a cut file must be refused")
	}
	if _, err := Open(bytes.NewReader([]byte("not an index at all, nothing in it......")), 40); err == nil {
		t.Error("a file with another header must be refused")
	}
}
