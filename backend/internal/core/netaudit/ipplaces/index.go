// Package ipplaces says roughly where an internet address is used (a city and a region), from the public DB-IP Lite City table (CC BY 4.0). The
// table is large (about 86 MB compressed, 7.8 million ranges), so it is never held in memory: a download is turned into one compact index file and
// an address is looked up by binary search straight on that file, a few dozen small reads. Memory use is a few kilobytes whatever the table holds.
// Nothing an address talked to is ever sent anywhere to be looked up (Constitution I.13). The names are text somebody else chose, so they are
// made plain and bounded here and shown only as text.
package ipplaces

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net/netip"
	"sort"
)

const (
	headerSize = 32
	// magic names the version: 02 added the country and the point to each place, so a first-version file is refused and downloaded again.
	magic      = "IPCITY02"
	v4Record   = 8  // first address (4 bytes) and place id (4)
	v6Record   = 12 // the first 8 bytes of the first address and place id (4)
	offsetSize = 4
	// none is the place id of a stretch that has no place: a gap, an unknown range, or a range with no city or region.
	none = ^uint32(0)
)

// Place is where an address is used. Region or City may be empty; a range with neither has no place. Country is the table's two capital
// letters for the place (empty when it gave none), and Lat and Lon are the city's point in degrees when Located: approximate, like the city.
type Place struct {
	Country string
	Region  string
	City    string
	Lat     float64
	Lon     float64
	Located bool
}

const (
	// pointScale stores a degree in a whole number of ten-thousandths (about 11 m), four bytes each.
	pointScale = 1e4
	pointBytes = 8
	// noPoint marks a place whose table row gave no usable point.
	noPoint      = math.MinInt32
	countryBytes = 2
)

// Counts says what an index holds.
type Counts struct {
	V4, V6, Places int
}

// Ranges is every run of addresses the index tells apart.
func (c Counts) Ranges() int { return c.V4 + c.V6 }

type layout struct {
	counts  Counts
	blobLen int
	v6At    int64
	offAt   int64
	blobAt  int64
	size    int64
}

func layoutOf(v4, v6, places, blobLen int) layout {
	l := layout{counts: Counts{V4: v4, V6: v6, Places: places}, blobLen: blobLen}
	l.v6At = headerSize + int64(v4)*v4Record
	l.offAt = l.v6At + int64(v6)*v6Record
	l.blobAt = l.offAt + int64(places+1)*offsetSize
	l.size = l.blobAt + int64(blobLen)
	return l
}

// Index is an opened index file. It is safe for many goroutines: it only reads.
type Index struct {
	r io.ReaderAt
	l layout
}

// ErrNotAnIndex is returned for a file that is not an index of this version, or is cut short.
var ErrNotAnIndex = errors.New("the places table is not a valid index")

// Open reads the header of an index of `size` bytes and checks that the sections fit the file.
func Open(r io.ReaderAt, size int64) (*Index, error) {
	var h [headerSize]byte
	if _, err := r.ReadAt(h[:], 0); err != nil || string(h[:len(magic)]) != magic {
		return nil, ErrNotAnIndex
	}
	v4, v6 := int(binary.BigEndian.Uint32(h[8:])), int(binary.BigEndian.Uint32(h[12:]))
	places, blob := int(binary.BigEndian.Uint32(h[16:])), int(binary.BigEndian.Uint32(h[20:]))
	l := layoutOf(v4, v6, places, blob)
	if v4 < 0 || v6 < 0 || places < 0 || blob < 0 || l.size != size {
		return nil, ErrNotAnIndex
	}
	return &Index{r: r, l: l}, nil
}

// Counts says what the index holds.
func (x *Index) Counts() Counts { return x.l.counts }

// Lookup says where an address is used. An address in no range, or in one with no city or region, has no place.
func (x *Index) Lookup(a netip.Addr) (Place, bool, error) {
	a = a.Unmap()
	var id uint32
	var err error
	if a.Is4() {
		b := a.As4()
		id, err = x.find(headerSize, x.l.counts.V4, v4Record, uint64(binary.BigEndian.Uint32(b[:])), 4)
	} else {
		b := a.As16()
		id, err = x.find(x.l.v6At, x.l.counts.V6, v6Record, binary.BigEndian.Uint64(b[:8]), 8)
	}
	if err != nil || id == none {
		return Place{}, false, err
	}
	p, err := x.place(id)
	return p, err == nil, err
}

// find is a binary search for the last record whose first address is not after key; the place id of that record, or none.
func (x *Index) find(at int64, n, size int, key uint64, keyLen int) (uint32, error) {
	var buf [v6Record]byte
	var rerr error
	read := func(i int) uint64 {
		if _, err := x.r.ReadAt(buf[:size], at+int64(i)*int64(size)); err != nil && rerr == nil {
			rerr = err
		}
		if keyLen == 4 {
			return uint64(binary.BigEndian.Uint32(buf[:4]))
		}
		return binary.BigEndian.Uint64(buf[:8])
	}
	i := sort.Search(n, func(i int) bool { return read(i) > key })
	if rerr != nil {
		return none, fmt.Errorf("the places table could not be read: %w", rerr)
	}
	if i == 0 {
		return none, nil
	}
	if _, err := x.r.ReadAt(buf[:size], at+int64(i-1)*int64(size)); err != nil {
		return none, fmt.Errorf("the places table could not be read: %w", err)
	}
	return binary.BigEndian.Uint32(buf[size-4 : size]), nil
}

// place reads one place: two bytes of country (zeros for none), the region, a zero byte, the city, and the point (latitude and longitude, four
// bytes each).
func (x *Index) place(id uint32) (Place, error) {
	if int(id) >= x.l.counts.Places {
		return Place{}, ErrNotAnIndex
	}
	var o [2 * offsetSize]byte
	if _, err := x.r.ReadAt(o[:], x.l.offAt+int64(id)*offsetSize); err != nil {
		return Place{}, fmt.Errorf("the places table could not be read: %w", err)
	}
	start, end := binary.BigEndian.Uint32(o[:4]), binary.BigEndian.Uint32(o[4:])
	if end < start || int(end) > x.l.blobLen {
		return Place{}, ErrNotAnIndex
	}
	raw := make([]byte, end-start)
	if _, err := x.r.ReadAt(raw, x.l.blobAt+int64(start)); err != nil {
		return Place{}, fmt.Errorf("the places table could not be read: %w", err)
	}
	return decodePlace(raw)
}

// decodePlace reads one place's bytes (see place).
func decodePlace(raw []byte) (Place, error) {
	if len(raw) < countryBytes+1+pointBytes {
		return Place{}, ErrNotAnIndex
	}
	var p Place
	if raw[0] != 0 {
		p.Country = string(raw[:countryBytes])
	}
	text, point := raw[countryBytes:len(raw)-pointBytes], raw[len(raw)-pointBytes:]
	sep := bytes.IndexByte(text, 0)
	if sep < 0 {
		return Place{}, ErrNotAnIndex
	}
	p.Region, p.City = string(text[:sep]), string(text[sep+1:])
	lat, lon := int32(binary.BigEndian.Uint32(point[:4])), int32(binary.BigEndian.Uint32(point[4:]))
	if lat != noPoint {
		p.Lat, p.Lon, p.Located = float64(lat)/pointScale, float64(lon)/pointScale, true
	}
	return p, nil
}
