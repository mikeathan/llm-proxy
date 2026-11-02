// Package oui names the maker of a network card from the first three bytes of its hardware (MAC) address, using the IEEE
// registry of 24-bit assignments bundled into the program (oui.tsv.gz, produced by tools/gen-oui). It starts no program and
// makes no request: the lookup is a map read. The text it returns comes from the bundled registry, never from a device.
package oui

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/csv"
	"errors"
	"io"
	"sort"
	"strings"
	"sync"
)

//go:embed oui.tsv.gz
var bundled []byte

type table map[string]string // six upper-case hex digits -> maker

var (
	once   sync.Once
	loaded table
)

// Vendor returns the maker of a card for a MAC address in colon, dash or bare spelling, or "" when the address is
// malformed, randomized (a private address says nothing about the card) or not in the registry.
func Vendor(mac string) string {
	once.Do(func() {
		zr, err := gzip.NewReader(bytes.NewReader(bundled))
		if err == nil {
			loaded, _ = parse(zr)
		}
	})
	return loaded.vendor(mac)
}

func parse(r io.Reader) (table, error) {
	t := table{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		key, name, ok := strings.Cut(sc.Text(), "\t")
		if ok && len(key) == 6 {
			t[key] = name
		}
	}
	return t, sc.Err()
}

func (t table) vendor(mac string) string {
	hex := strings.ToUpper(strings.NewReplacer(":", "", "-", "").Replace(mac))
	if len(hex) != 12 {
		return ""
	}
	first := hexVal(hex[1]) // the locally administered bit is 0x02 of the first byte, so of its second digit
	if first < 0 || first&0x2 != 0 {
		return ""
	}
	return t[hex[:6]]
}

func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

// Convert reads the IEEE registry file (oui.csv) and writes the bundled form: gzip of "ASSIGNMENT<TAB>NAME" lines sorted by
// assignment. Only the 24-bit (MA-L) rows with a name are kept; entries named "Private" are left out.
func Convert(csvFile io.Reader, out io.Writer) error {
	rd := csv.NewReader(csvFile)
	rd.FieldsPerRecord = -1
	rd.LazyQuotes = true
	rows := map[string]string{}
	for {
		rec, err := rd.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if len(rec) < 3 || rec[0] != "MA-L" || len(rec[1]) != 6 {
			continue
		}
		name := strings.Join(strings.Fields(rec[2]), " ")
		if name == "" || strings.EqualFold(name, "Private") {
			continue
		}
		rows[strings.ToUpper(rec[1])] = name
	}
	keys := make([]string, 0, len(rows))
	for k := range rows {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	zw, _ := gzip.NewWriterLevel(out, gzip.BestCompression)
	for _, k := range keys {
		if _, err := io.WriteString(zw, k+"\t"+rows[k]+"\n"); err != nil {
			return err
		}
	}
	return zw.Close()
}
