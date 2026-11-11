package ipplaces

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/netip"

	"llm-proxy/internal/core/netaudit/ipowners"
)

const (
	// columns of a row: first address, last address, continent, country, region, city, latitude, longitude.
	columns        = 8
	colFirst       = 0
	colLast        = 1
	colRegion      = 4
	colCity        = 5
	maxNameRunes   = 80
	maxPlaces      = 1 << 22
	maxRows        = 20_000_000
	unknownCountry = "ZZ"
	colCountry     = 3
)

// family is where the run being written of one address family has got to.
type family struct {
	has bool   // a row of this family has been seen
	end uint64 // the last address (key) of the run
	id  uint32 // its place, or none
	max uint64 // the largest key the family has
}

// builder writes the records of one pass over the table as they come, so no more than the places (a few hundred thousand short strings) is ever held.
type builder struct {
	out     *bufio.Writer
	ids     map[string]uint32
	offsets []uint32
	blob    bytes.Buffer
	v4, v6  int
	f4, f6  family
	inV6    bool
}

// Build reads the table (CSV: first and last address, continent, country, region, city, latitude, longitude; IPv4 rows first, each family in
// order) and writes the index to w. A row with no known country, no city and no region, an overlapping row and a row that is not a row are skipped;
// neighbouring rows of one place are joined into one run. A table with no usable row is an error: an empty index must not read as "nowhere".
func Build(r io.Reader, w io.WriterAt) (Counts, error) {
	b := &builder{out: bufio.NewWriter(io.NewOffsetWriter(w, headerSize)), ids: map[string]uint32{}, offsets: []uint32{0}, f4: family{max: 1<<32 - 1}, f6: family{max: ^uint64(0)}}
	cr := csv.NewReader(bufio.NewReaderSize(r, 1<<16))
	cr.FieldsPerRecord = -1
	cr.ReuseRecord = true
	for rows := 0; ; rows++ {
		if rows >= maxRows {
			return Counts{}, fmt.Errorf("the places table holds more than %d rows", maxRows)
		}
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			var pe *csv.ParseError
			if errors.As(err, &pe) {
				continue
			}
			return Counts{}, fmt.Errorf("the places table could not be read: %w", err)
		}
		if err := b.add(rec); err != nil {
			return Counts{}, err
		}
	}
	if err := b.closeRun(true); err != nil {
		return Counts{}, err
	}
	if err := b.closeRun(false); err != nil {
		return Counts{}, err
	}
	if b.v4+b.v6 == 0 {
		return Counts{}, errors.New("the places table holds no usable range")
	}
	return b.finish(w)
}

// row is one usable line of the table.
type row struct {
	v4         bool
	start, end uint64
	known      bool // the country is known
	region     string
	city       string
}

// parseRow reads one line, or says it is not a usable row: too few columns, addresses that are not addresses or are of two families, or a
// range that ends before it starts.
func parseRow(rec []string) (row, bool) {
	if len(rec) < columns {
		return row{}, false
	}
	first, err1 := netip.ParseAddr(rec[colFirst])
	last, err2 := netip.ParseAddr(rec[colLast])
	if err1 != nil || err2 != nil || first.Is4() != last.Is4() || last.Less(first) {
		return row{}, false
	}
	return row{
		v4: first.Is4(), start: keyOf(first), end: keyOf(last), known: rec[colCountry] != unknownCountry,
		region: ipowners.PlainText(rec[colRegion], maxNameRunes), city: ipowners.PlainText(rec[colCity], maxNameRunes),
	}, true
}

// enter notes which family the rows are in; the first IPv6 row ends the IPv4 rows, and an IPv4 row after it is an error.
func (b *builder) enter(v4 bool) error {
	if v4 && b.inV6 {
		return errors.New("the places table is not in order: an IPv4 row follows an IPv6 row")
	}
	if v4 || b.inV6 {
		return nil
	}
	b.inV6 = true
	return b.closeRun(true)
}

// add reads one line.
func (b *builder) add(rec []string) error {
	r, ok := parseRow(rec)
	if !ok {
		return nil
	}
	if err := b.enter(r.v4); err != nil {
		return err
	}
	if f := b.family(r.v4); f.has && r.start <= f.end {
		return nil // inside the run before it: the file is not in order
	}
	id := none
	if r.known && (r.region != "" || r.city != "") {
		var err error
		if id, err = b.intern(r.region, r.city); err != nil {
			return err
		}
	}
	return b.run(r.v4, r.start, r.end, id)
}

// keyOf is what an address is ordered and looked up by: all 32 bits of an IPv4 address, the first 64 bits of an IPv6 one.
func keyOf(a netip.Addr) uint64 {
	if a.Is4() {
		b := a.As4()
		return uint64(binary.BigEndian.Uint32(b[:]))
	}
	b := a.As16()
	return binary.BigEndian.Uint64(b[:8])
}

func (b *builder) family(v4 bool) *family {
	if v4 {
		return &b.f4
	}
	return &b.f6
}

// run adds a stretch of addresses with one place. It joins the stretch to the run before it when it starts right after it and has the same place,
// ends the run before it with a gap record when a gap follows it.
func (b *builder) run(v4 bool, start, end uint64, id uint32) error {
	f := b.family(v4)
	before := none // the place just before this stretch, once a gap is accounted for
	if f.has && start == f.end+1 {
		before = f.id
	} else if f.has && f.id != none {
		if err := b.write(v4, f.end+1, none); err != nil {
			return err
		}
	}
	if id != before {
		if err := b.write(v4, start, id); err != nil {
			return err
		}
	}
	f.has, f.end, f.id = true, end, id
	return nil
}

// closeRun ends the last run of one family with a gap record, so an address after the table's last range has no place.
func (b *builder) closeRun(v4 bool) error {
	f := b.family(v4)
	if !f.has || f.id == none || f.end == f.max {
		return nil
	}
	f.id = none
	return b.write(v4, f.end+1, none)
}

// write adds one record.
func (b *builder) write(v4 bool, start uint64, id uint32) error {
	var rec [v6Record]byte
	var n int
	if v4 {
		binary.BigEndian.PutUint32(rec[:4], uint32(start))
		binary.BigEndian.PutUint32(rec[4:8], id)
		n = v4Record
		b.v4++
	} else {
		binary.BigEndian.PutUint64(rec[:8], start)
		binary.BigEndian.PutUint32(rec[8:12], id)
		n = v6Record
		b.v6++
	}
	if _, err := b.out.Write(rec[:n]); err != nil {
		return fmt.Errorf("the places index could not be written: %w", err)
	}
	return nil
}

// intern gives a place its id, storing its text once.
func (b *builder) intern(region, city string) (uint32, error) {
	key := region + "\x00" + city
	if id, ok := b.ids[key]; ok {
		return id, nil
	}
	if len(b.ids) >= maxPlaces {
		return 0, fmt.Errorf("the places table holds more than %d distinct places", maxPlaces)
	}
	id := uint32(len(b.ids))
	b.ids[key] = id
	b.blob.WriteString(key)
	b.offsets = append(b.offsets, uint32(b.blob.Len()))
	return id, nil
}

// finish writes the offsets and the text after the records, then the header.
func (b *builder) finish(w io.WriterAt) (Counts, error) {
	for _, o := range b.offsets {
		var u [offsetSize]byte
		binary.BigEndian.PutUint32(u[:], o)
		if _, err := b.out.Write(u[:]); err != nil {
			return Counts{}, fmt.Errorf("the places index could not be written: %w", err)
		}
	}
	if _, err := b.out.Write(b.blob.Bytes()); err != nil {
		return Counts{}, fmt.Errorf("the places index could not be written: %w", err)
	}
	if err := b.out.Flush(); err != nil {
		return Counts{}, fmt.Errorf("the places index could not be written: %w", err)
	}
	var h [headerSize]byte
	copy(h[:], magic)
	binary.BigEndian.PutUint32(h[8:], uint32(b.v4))
	binary.BigEndian.PutUint32(h[12:], uint32(b.v6))
	binary.BigEndian.PutUint32(h[16:], uint32(len(b.ids)))
	binary.BigEndian.PutUint32(h[20:], uint32(b.blob.Len()))
	if _, err := w.WriteAt(h[:], 0); err != nil {
		return Counts{}, fmt.Errorf("the places index could not be written: %w", err)
	}
	return Counts{V4: b.v4, V6: b.v6, Places: len(b.ids)}, nil
}
