// Command ouigen builds the maker table the Radio stack embeds (oui_data.tsv.gz) from the three IEEE registries: MA-L (24 bits), MA-M (28) and
// MA-S (36). Run it by hand when the table should be refreshed; the build never needs the network.
//
//	go run ./tools/ouigen -out oui_data.tsv.gz oui.csv mam.csv oui36.csv
//
// Each output line is the assignment in upper-case hex (6, 7 or 9 digits), a tab and a short maker name. An organisation the IEEE lists as
// "Private" is left out: it names nobody.
package main

import (
	"compress/gzip"
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

const (
	maxName    = 28
	privateOrg = "private"
)

// legalSuffixes are trailing words that add length and no meaning to a maker's name.
var legalSuffixes = map[string]bool{
	"inc": true, "corp": true, "corporation": true, "co": true, "ltd": true, "limited": true, "llc": true, "gmbh": true, "sa": true, "ag": true,
	"bv": true, "oy": true, "ab": true, "plc": true, "pte": true, "pty": true, "kg": true, "srl": true, "spa": true, "sdn": true, "bhd": true,
	"company": true, "incorporated": true, "lp": true, "nv": true, "as": true, "kk": true, "sas": true, "sarl": true,
}

// short trims an organisation's registered name to the name people know it by: no legal suffix, no stray punctuation, at most maxName bytes.
func short(org string) string {
	words := strings.Fields(strings.ReplaceAll(org, ",", " "))
	for len(words) > 1 && legalSuffixes[strings.ToLower(strings.Trim(words[len(words)-1], ".,()"))] {
		words = words[:len(words)-1]
	}
	name := strings.Trim(strings.Join(words, " "), " .,")
	if len(name) > maxName {
		cut := strings.LastIndex(name[:maxName], " ")
		if cut < 8 {
			cut = maxName
		}
		name = strings.TrimRight(name[:cut], " .,-")
	}
	return name
}

func readRegistry(path string, into map[string]string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	for _, row := range rows[1:] {
		if len(row) < 3 || strings.EqualFold(strings.TrimSpace(row[2]), privateOrg) {
			continue
		}
		if name := short(row[2]); name != "" {
			into[strings.ToUpper(strings.TrimSpace(row[1]))] = name
		}
	}
	return nil
}

func main() {
	out := flag.String("out", "oui_data.tsv.gz", "the file to write")
	flag.Parse()
	table := map[string]string{}
	for _, p := range flag.Args() {
		if err := readRegistry(p, table); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	keys := make([]string, 0, len(table))
	for k := range table {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	f, err := os.Create(*out)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	zw, _ := gzip.NewWriterLevel(f, gzip.BestCompression)
	for _, k := range keys {
		fmt.Fprintf(zw, "%s\t%s\n", k, table[k])
	}
	if err := zw.Close(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := f.Close(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("%d makers\n", len(keys))
}
