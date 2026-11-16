package radio

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"strings"
	"sync"
)

// The maker of a device, read from the first bytes of its address (the IEEE registry of who was assigned which block). It is derived once, when
// the frame is parsed, and only the maker's name is kept next to the keyed tag: the address is not (ADR-0057, Constitution I.16). A device with a
// random address (a phone or a laptop, by design) has no maker, and says so.
//
// The table is oui_data.tsv.gz, built by tools/ouigen from the IEEE's MA-L, MA-M and MA-S registries and embedded here, so no lookup ever leaves
// the computer. Refresh it by running the generator and committing the file.

//go:embed oui_data.tsv.gz
var ouiData []byte

// Digits of the three assignment sizes: MA-L (24 bits), MA-M (28) and MA-S (36).
const (
	ouiDigitsL = 6
	ouiDigitsM = 7
	ouiDigitsS = 9
)

// makers loads the table once. A damaged table gives an empty one: the rows then read "maker unknown", never a wrong name.
var makers = sync.OnceValue(func() map[string]string {
	out := map[string]string{}
	zr, err := gzip.NewReader(bytes.NewReader(ouiData))
	if err != nil {
		return out
	}
	defer zr.Close()
	sc := bufio.NewScanner(zr)
	for sc.Scan() {
		if k, v, ok := strings.Cut(sc.Text(), "\t"); ok {
			out[k] = v
		}
	}
	return out
})

// MakerOf is the maker the address's block was assigned to, or "" for a group address, a random (locally administered) address, a block the
// registry keeps private and a block it does not list. The longest assignment wins: a 36-bit block is more specific than the 24-bit one around it.
func MakerOf(m MAC) string {
	if m[0]&1 != 0 || m[0]&2 != 0 {
		return ""
	}
	hex := fmt.Sprintf("%02X%02X%02X%02X%02X", m[0], m[1], m[2], m[3], m[4])
	table := makers()
	for _, n := range []int{ouiDigitsS, ouiDigitsM, ouiDigitsL} {
		if name, ok := table[hex[:n]]; ok {
			return name
		}
	}
	return ""
}
