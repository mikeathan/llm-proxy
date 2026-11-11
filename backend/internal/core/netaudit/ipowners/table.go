// Package ipowners says which network an internet address belongs to, from a public table of address ranges (IPtoASN, public domain) that is
// downloaded when the operator asks and looked up on this computer: no address a device talked to is ever sent anywhere to be looked up
// (Constitution I.13). A row gives the range, the autonomous system number, a two-letter country and the holder's name. The name is text the
// holder chose, so it is made plain and bounded here and shown only as text.
package ipowners

import (
	"bufio"
	"cmp"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"llm-proxy/models"
)

const (
	// maxNameRunes bounds a holder's name.
	maxNameRunes = 100
	maxLine      = 1024
	// columns of a row: first address, last address, AS number, country, holder.
	columns = 5
	// notRouted is the AS number the table gives to ranges nobody announces; they have no owner to show.
	notRouted  = 0
	countryLen = 2
)

// maxRows bounds the rows of a table (both families together): the real one holds about 720,000. A larger file is refused, not cut. It is a
// variable only so a test can lower it.
var maxRows = 1_500_000

type row4 struct {
	start, end uint32
	asn        uint32
	country    [countryLen]byte
	org        uint32
}

type row6 struct {
	start, end [16]byte
	asn        uint32
	country    [countryLen]byte
	org        uint32
}

// Table is the parsed ranges, sorted for a binary search, with the holders' names stored once each.
type Table struct {
	v4   []row4
	v6   []row6
	orgs []string
}

// Counts says how many ranges are held for each address family.
func (t *Table) Counts() (v4, v6 int) {
	if t == nil {
		return 0, 0
	}
	return len(t.v4), len(t.v6)
}

// Lookup says who owns an address. An address in no range, in a range nobody announces, or a private, loopback or other local address has no
// owner. A nil table has none either.
func (t *Table) Lookup(a netip.Addr) (models.TrafficOwner, bool) {
	if t == nil {
		return models.TrafficOwner{}, false
	}
	a = a.Unmap()
	if a.Is4() {
		return t.lookup4(a)
	}
	return t.lookup6(a)
}

func (t *Table) lookup4(a netip.Addr) (models.TrafficOwner, bool) {
	b := a.As4()
	n := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	i, _ := slices.BinarySearchFunc(t.v4, n, func(r row4, n uint32) int { return cmp.Compare(r.start, n) })
	// i is the first range that starts after n, or at n; the candidate is that one when it starts at n, else the one before.
	if i < len(t.v4) && t.v4[i].start == n {
		return t.owner(t.v4[i].asn, t.v4[i].country, t.v4[i].org), true
	}
	if i == 0 || t.v4[i-1].end < n {
		return models.TrafficOwner{}, false
	}
	r := t.v4[i-1]
	return t.owner(r.asn, r.country, r.org), true
}

func (t *Table) lookup6(a netip.Addr) (models.TrafficOwner, bool) {
	b := a.As16()
	i, found := slices.BinarySearchFunc(t.v6, b, func(r row6, b [16]byte) int { return slices.Compare(r.start[:], b[:]) })
	if found {
		return t.owner(t.v6[i].asn, t.v6[i].country, t.v6[i].org), true
	}
	if i == 0 || slices.Compare(t.v6[i-1].end[:], b[:]) < 0 {
		return models.TrafficOwner{}, false
	}
	r := t.v6[i-1]
	return t.owner(r.asn, r.country, r.org), true
}

func (t *Table) owner(asn uint32, country [countryLen]byte, org uint32) models.TrafficOwner {
	o := models.TrafficOwner{ASN: asn, Name: t.orgs[org]}
	if country != [countryLen]byte{} {
		o.Country = string(country[:])
	}
	return o
}

// builder collects rows while a file is read.
type builder struct {
	v4    []row4
	v6    []row6
	orgs  []string
	index map[string]uint32
}

func (b *builder) intern(name string) uint32 {
	if i, ok := b.index[name]; ok {
		return i
	}
	i := uint32(len(b.orgs))
	b.orgs = append(b.orgs, name)
	b.index[name] = i
	return i
}

// Parse reads a table: one range a line, tab separated (first address, last address, AS number, country, holder). A line that is not one of
// these, a range that ends before it starts, a range that mixes families and a range nobody announces are skipped, and a country that is not two
// capital letters is dropped from its row. A table with no usable row is an error: an empty table must not read as "no one owns anything".
func Parse(r io.Reader) (*Table, error) { return ParseAll(r) }

// ParseAll reads several files into one table. Each must hold a usable range by itself, so a file that is empty or wrong cannot hide behind the
// others, and the bound on rows is for all of them together.
func ParseAll(files ...io.Reader) (*Table, error) {
	b := &builder{index: map[string]uint32{}}
	for _, r := range files {
		before := len(b.v4) + len(b.v6)
		if err := b.read(r); err != nil {
			return nil, err
		}
		if len(b.v4)+len(b.v6) == before {
			return nil, errors.New("the table holds no usable range")
		}
	}
	return b.finish(), nil
}

func (b *builder) read(r io.Reader) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 4096), maxLine)
	for sc.Scan() {
		if len(b.v4)+len(b.v6) >= maxRows {
			return fmt.Errorf("the table holds more than %d ranges", maxRows)
		}
		b.add(sc.Text())
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("the table could not be read: %w", err)
	}
	return nil
}

// add reads one line into the rows it makes.
func (b *builder) add(line string) {
	r, ok := parseRow(line)
	if !ok {
		return
	}
	org := b.intern(plainName(r.holder))
	if r.first.Is4() {
		s, e := r.first.As4(), r.last.As4()
		b.v4 = append(b.v4, row4{start: be32(s), end: be32(e), asn: r.asn, country: r.country, org: org})
		return
	}
	b.v6 = append(b.v6, row6{start: r.first.As16(), end: r.last.As16(), asn: r.asn, country: r.country, org: org})
}

// textRow is one line of a table, read but not yet stored.
type textRow struct {
	first, last netip.Addr
	asn         uint32
	country     [countryLen]byte
	holder      string
}

// parseRow reads one line, or says it is not a usable row: a comment, too few columns, addresses that are not addresses or are of two families,
// a range that ends before it starts, and a range nobody announces. A country that is not two capital letters is left out of the row.
func parseRow(line string) (textRow, bool) {
	if line == "" || line[0] == '#' {
		return textRow{}, false
	}
	f := strings.SplitN(line, "\t", columns)
	if len(f) < columns {
		return textRow{}, false
	}
	first, err1 := netip.ParseAddr(f[0])
	last, err2 := netip.ParseAddr(f[1])
	asn, err3 := strconv.ParseUint(f[2], 10, 32)
	if err1 != nil || err2 != nil || err3 != nil || asn == notRouted || first.Is4() != last.Is4() || last.Less(first) {
		return textRow{}, false
	}
	return textRow{first: first, last: last, asn: uint32(asn), country: countryOf(f[3]), holder: f[4]}, true
}

// countryOf is the two capital letters of a country column, or nothing.
func countryOf(c string) [countryLen]byte {
	if len(c) == countryLen && isCapital(c[0]) && isCapital(c[1]) {
		return [countryLen]byte{c[0], c[1]}
	}
	return [countryLen]byte{}
}

func be32(b [4]byte) uint32 {
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

func isCapital(c byte) bool { return c >= 'A' && c <= 'Z' }

// finish sorts each family and drops a range that overlaps the one before it, so that the binary search is sound whatever the file held.
func (b *builder) finish() *Table {
	slices.SortStableFunc(b.v4, func(x, y row4) int { return cmp.Compare(x.start, y.start) })
	slices.SortStableFunc(b.v6, func(x, y row6) int { return slices.Compare(x.start[:], y.start[:]) })
	v4 := b.v4[:0]
	for _, r := range b.v4 {
		if len(v4) == 0 || r.start > v4[len(v4)-1].end {
			v4 = append(v4, r)
		}
	}
	v6 := b.v6[:0]
	for _, r := range b.v6 {
		if len(v6) == 0 || slices.Compare(r.start[:], v6[len(v6)-1].end[:]) > 0 {
			v6 = append(v6, r)
		}
	}
	return &Table{v4: slices.Clip(v4), v6: slices.Clip(v6), orgs: b.orgs}
}

// plainName makes a holder's name plain text: control, format (including the right-to-left overrides) and unprintable characters go, white space
// is collapsed, and at most maxNameRunes characters are kept.
func plainName(raw string) string {
	var out strings.Builder
	runes, space := 0, false
	for _, r := range raw {
		switch {
		case r == utf8.RuneError, unicode.IsControl(r), unicode.Is(unicode.Cf, r), !unicode.IsPrint(r) && !unicode.IsSpace(r):
			continue
		case unicode.IsSpace(r):
			space = out.Len() > 0
			continue
		}
		if space {
			out.WriteByte(' ')
			runes++
			space = false
		}
		if runes >= maxNameRunes {
			break
		}
		out.WriteRune(r)
		runes++
	}
	return strings.TrimSpace(out.String())
}
