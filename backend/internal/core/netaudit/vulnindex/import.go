// Package vulnindex builds the local vulnerability index from the two files an operator downloads: NVD's CVE data
// (which products and versions each CVE affects) and CISA's Known Exploited Vulnerabilities catalogue. It only reads
// files it is handed; it never fetches anything. Text from the files is untrusted, so only validated CVE ids, product
// names, versions and scores are kept, and no description is ever stored.
package vulnindex

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"llm-proxy/models"
)

const (
	// maxDecompressed bounds what is read from one NVD file once decompressed (a recent year is about 0.5 GB).
	maxDecompressed = 2 << 30
	// maxKEVBytes bounds the catalogue (about 2 MB today).
	maxKEVBytes  = 64 << 20
	gzipMagic0   = 0x1f
	gzipMagic1   = 0x8b
	statusReject = "Rejected"
)

// maxEntries bounds the product ranges one import may add; a variable so a test can lower it.
var maxEntries = 5_000_000

var (
	// ErrBadFile means a file is not the data it was said to be; the index is left as it was.
	ErrBadFile = errors.New("not a usable vulnerability data file")
	// ErrTooLarge means an import would add more than the index accepts.
	ErrTooLarge = errors.New("the file holds more entries than the index accepts")

	cvePattern  = regexp.MustCompile(`^CVE-\d{4}-\d{4,}$`)
	datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

// The slice of NVD's CVE 2.0 format the importer reads. Everything else in the file is skipped.
type nvdItem struct {
	CVE struct {
		ID             string `json:"id"`
		VulnStatus     string `json:"vulnStatus"`
		Metrics        map[string][]nvdMetric
		Configurations []nvdConfig `json:"configurations"`
	} `json:"cve"`
}

type nvdMetric struct {
	CVSSData struct {
		BaseScore float64 `json:"baseScore"`
	} `json:"cvssData"`
}

type nvdConfig struct {
	Operator string    `json:"operator"`
	Nodes    []nvdNode `json:"nodes"`
}

type nvdNode struct {
	Operator string     `json:"operator"`
	Negate   bool       `json:"negate"`
	CPEMatch []nvdMatch `json:"cpeMatch"`
}

type nvdMatch struct {
	Vulnerable bool   `json:"vulnerable"`
	Criteria   string `json:"criteria"`
	StartIncl  string `json:"versionStartIncluding"`
	StartExcl  string `json:"versionStartExcluding"`
	EndIncl    string `json:"versionEndIncluding"`
	EndExcl    string `json:"versionEndExcluding"`
}

// scoreOrder is the order CVSS versions are tried in: the newest score that exists is the one kept.
var scoreOrder = []string{"cvssMetricV40", "cvssMetricV31", "cvssMetricV30", "cvssMetricV2"}

type keyedRange struct {
	key string
	models.VulnRange
}

// ImportNVD reads one NVD CVE 2.0 file (JSON, or gzip as saved) and returns base with that file's CVEs merged in: a CVE
// the file holds replaces any earlier entries for the same CVE id, so importing a year again refreshes it, and other
// CVEs are kept. base is never changed, and a bad file returns base's data untouched with an error.
func ImportNVD(base models.VulnIndex, r io.Reader, now time.Time) (models.VulnIndex, error) {
	in, err := nvdReader(r)
	if err != nil {
		return base, err
	}
	var added []keyedRange
	seen := map[string]bool{}
	src := models.VulnSource{ImportedAt: now}
	err = eachItem(in, func(it nvdItem) error {
		id := it.CVE.ID
		if !cvePattern.MatchString(id) {
			return nil
		}
		seen[id] = true
		if it.CVE.VulnStatus == statusReject {
			src.Rejected++
			return nil
		}
		if len(it.CVE.Configurations) == 0 {
			src.NoConfig++
			return nil
		}
		entries, needsAll := entriesOf(it)
		if len(entries) == 0 && needsAll {
			src.NeedsAll++
		}
		added = append(added, entries...)
		if len(added) > maxEntries {
			return ErrTooLarge
		}
		return nil
	})
	if err != nil {
		return base, err
	}
	out := base
	out.Entries = mergeEntries(base.Entries, added, seen)
	src.CVEs, src.Entries = countOf(out.Entries)
	out.NVD = src
	return out, nil
}

// nvdReader returns the file's JSON, decompressing it when it is gzip, and bounds how much is read.
func nvdReader(r io.Reader) (io.Reader, error) {
	br := bufio.NewReaderSize(r, 1<<16)
	head, err := br.Peek(2)
	if err != nil {
		return nil, fmt.Errorf("%w: the file is empty", ErrBadFile)
	}
	if head[0] == gzipMagic0 && head[1] == gzipMagic1 {
		zr, err := gzip.NewReader(br)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrBadFile, err)
		}
		return io.LimitReader(zr, maxDecompressed), nil
	}
	return io.LimitReader(br, maxDecompressed), nil
}

// eachItem streams the "vulnerabilities" array of an NVD file, calling fn for each element, so a file of hundreds of
// megabytes never sits in memory whole.
func eachItem(r io.Reader, fn func(nvdItem) error) error {
	dec := json.NewDecoder(r)
	if err := expectDelim(dec, '{'); err != nil {
		return err
	}
	found := false
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return badFile(err)
		}
		if key != "vulnerabilities" {
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				return badFile(err)
			}
			continue
		}
		found = true
		if err := eachListItem(dec, fn); err != nil {
			return err
		}
	}
	if _, err := dec.Token(); err != nil {
		return badFile(err)
	}
	if !found {
		return fmt.Errorf("%w: no \"vulnerabilities\" list", ErrBadFile)
	}
	return nil
}

// eachListItem reads a JSON array of items, calling fn for each, and the array's closing bracket.
func eachListItem(dec *json.Decoder, fn func(nvdItem) error) error {
	if err := expectDelim(dec, '['); err != nil {
		return err
	}
	for dec.More() {
		var it nvdItem
		if err := dec.Decode(&it); err != nil {
			return badFile(err)
		}
		if err := fn(it); err != nil {
			return err
		}
	}
	if _, err := dec.Token(); err != nil {
		return badFile(err)
	}
	return nil
}

func expectDelim(dec *json.Decoder, want json.Delim) error {
	tok, err := dec.Token()
	if err != nil {
		return badFile(err)
	}
	if tok != want {
		return fmt.Errorf("%w: expected %q", ErrBadFile, rune(want))
	}
	return nil
}

func badFile(err error) error {
	return fmt.Errorf("%w: %v", ErrBadFile, err)
}

// entriesOf turns one CVE's configurations into index entries. Configurations are alternatives. Inside one, only the
// matches marked vulnerable count: a "running on" platform node is not the flawed software. When a rule needs two
// vulnerable parts at once (an AND of two vulnerable groups) it cannot be matched from one service, so it adds nothing
// and needsAll is set.
func entriesOf(it nvdItem) (entries []keyedRange, needsAll bool) {
	score := scoreOf(it)
	for _, conf := range it.CVE.Configurations {
		var vulnerableNodes int
		var fromConf []keyedRange
		for _, n := range conf.Nodes {
			if n.Negate {
				continue
			}
			ms := vulnerableMatches(n)
			if len(ms) == 0 {
				continue
			}
			vulnerableNodes++
			if n.Operator == "AND" && len(ms) > 1 {
				needsAll = true
				continue
			}
			for _, m := range ms {
				if e, ok := entryOf(it.CVE.ID, score, m); ok {
					fromConf = append(fromConf, e)
				}
			}
		}
		if conf.Operator == "AND" && vulnerableNodes > 1 {
			needsAll = true
			continue
		}
		entries = append(entries, fromConf...)
	}
	return entries, needsAll
}

func vulnerableMatches(n nvdNode) []nvdMatch {
	var out []nvdMatch
	for _, m := range n.CPEMatch {
		if m.Vulnerable {
			out = append(out, m)
		}
	}
	return out
}

// scoreOf is the CVSS base score of a CVE (newest version available), or 0 when it has none or the value is not a
// score.
func scoreOf(it nvdItem) float64 {
	for _, k := range scoreOrder {
		if ms := it.CVE.Metrics[k]; len(ms) > 0 {
			if s := ms[0].CVSSData.BaseScore; s >= 0 && s <= 10 {
				return s
			}
			return 0
		}
	}
	return 0
}

// entryOf validates one match and turns it into an entry. Anything out of shape drops the match, not the CVE.
func entryOf(cve string, score float64, m nvdMatch) (keyedRange, bool) {
	f := splitCPE23(m.Criteria)
	if len(f) < 6 || f[0] != "cpe" || f[1] != "2.3" || !validKey(f[2], f[3], f[4]) {
		return keyedRange{}, false
	}
	r := models.VulnRange{CVE: cve, Score: score}
	switch version := f[5]; version {
	case "-":
		return keyedRange{}, false
	case "*":
	default:
		if v := normaliseVersion(version + updateOf(f)); versionPattern.MatchString(v) {
			r.Exact = v
		} else {
			return keyedRange{}, false
		}
	}
	for _, b := range []struct {
		in  string
		out *string
	}{{m.StartIncl, &r.StartIncl}, {m.StartExcl, &r.StartExcl}, {m.EndIncl, &r.EndIncl}, {m.EndExcl, &r.EndExcl}} {
		if b.in == "" {
			continue
		}
		v := normaliseVersion(b.in)
		if !versionPattern.MatchString(v) {
			return keyedRange{}, false
		}
		*b.out = v
	}
	return keyedRange{key: Key(f[2], f[3], f[4]), VulnRange: r}, true
}

// updateOf is the CPE 2.3 "update" field when it names a release of the version (OpenSSH 9.9 with update p1 is 9.9p1, the
// way a banner and nmap write it), and nothing for the wildcard and "not applicable" values.
func updateOf(f []string) string {
	if len(f) <= 6 || f[6] == "*" || f[6] == "-" {
		return ""
	}
	return f[6]
}

// normaliseVersion removes the 2.3 backslash escapes from a version but keeps its case.
func normaliseVersion(s string) string {
	return strings.ReplaceAll(s, `\`, "")
}

// mergeEntries returns a new map: the old entries minus those of any CVE in replaced, plus added. The old map and its
// slices are not changed.
func mergeEntries(old map[string][]models.VulnRange, added []keyedRange, replaced map[string]bool) map[string][]models.VulnRange {
	out := make(map[string][]models.VulnRange, len(old))
	for k, rs := range old {
		kept := slices.DeleteFunc(slices.Clone(rs), func(r models.VulnRange) bool { return replaced[r.CVE] })
		if len(kept) > 0 {
			out[k] = kept
		}
	}
	for _, e := range added {
		out[e.key] = append(out[e.key], e.VulnRange)
	}
	return out
}

// countOf is how many CVEs and how many product ranges an entry map holds.
func countOf(entries map[string][]models.VulnRange) (cves, ranges int) {
	ids := map[string]struct{}{}
	for _, rs := range entries {
		ranges += len(rs)
		for _, r := range rs {
			ids[r.CVE] = struct{}{}
		}
	}
	return len(ids), ranges
}

type kevCatalogue struct {
	Vulnerabilities []struct {
		CVEID     string `json:"cveID"`
		DateAdded string `json:"dateAdded"`
	} `json:"vulnerabilities"`
}

// ImportKEV reads CISA's catalogue (JSON) and returns base with its exploited list replaced by the catalogue's: an entry
// CISA no longer lists is no longer marked. Only the CVE id and the date added are kept. A bad file returns base with an
// error.
func ImportKEV(base models.VulnIndex, r io.Reader, now time.Time) (models.VulnIndex, error) {
	var cat kevCatalogue
	if err := json.NewDecoder(io.LimitReader(r, maxKEVBytes)).Decode(&cat); err != nil {
		return base, fmt.Errorf("%w: %v", ErrBadFile, err)
	}
	if cat.Vulnerabilities == nil {
		return base, fmt.Errorf("%w: no \"vulnerabilities\" list", ErrBadFile)
	}
	exploited := map[string]string{}
	for _, v := range cat.Vulnerabilities {
		if !cvePattern.MatchString(v.CVEID) {
			continue
		}
		date := v.DateAdded
		if !datePattern.MatchString(date) {
			date = ""
		}
		exploited[v.CVEID] = date
	}
	out := base
	out.Exploited = maps.Clone(exploited)
	out.KEV = models.VulnSource{ImportedAt: now, CVEs: len(exploited)}
	return out, nil
}
