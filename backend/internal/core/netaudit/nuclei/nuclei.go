// Package nuclei is the guarded way to run the nuclei web checker (Constitution II.3, guarded programs). The model never supplies a
// flag: every argument comes from the fixed table below, from a target built in code, or from the path of a template whose file was
// checked against a reviewed checksum. Only HTTP templates run, nothing is downloaded or updated by the program, no out-of-band
// callback server is contacted, and what comes back is parsed into typed facts and reported in this repository's own words.
//
// The flags below were read from `nuclei -h` of v3.11.1 (2026-11-05).
package nuclei

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// The bounds of one run.
const (
	MaxTargets   = 64
	MaxTemplates = 64
	// maxRunTime is the longest a run may be given; the caller chooses less.
	maxRunTime = time.Hour
	// maxLine is the longest result line read; a longer one is skipped.
	maxLine = 64 << 10
	// maxResponse is how much of an answer the program reads, in bytes.
	maxResponse = "262144"
)

// fixedFlags are on every run. -duc turns off the update check, -ni the callback server, -pt http limits it to HTTP templates, -dr
// ignores redirects (which could lead to another host), -nmhe keeps it from giving up on a device early; -j -or -ot make the output
// one JSON line per hit without the request, the answer or the template text; the rate, concurrency, bulk size, timeout, retries and
// response size are low and fixed so a device is touched gently.
var fixedFlags = []string{
	"-duc", "-ni", "-nc", "-silent", "-j", "-or", "-ot", "-dr", "-nmhe",
	"-pt", "http",
	"-rl", "5", "-c", "2", "-bs", "2", "-timeout", "8", "-retries", "0", "-rsr", maxResponse,
}

// Target is one web service on one device: an address already validated against the scan scope and a port.
type Target struct {
	Addr netip.Addr
	Port int
	TLS  bool
}

// URL is the only form a target takes on the command line.
func (t Target) URL() string {
	scheme := "http"
	if t.TLS {
		scheme = "https"
	}
	return scheme + "://" + netip.AddrPortFrom(t.Addr, uint16(t.Port)).String()
}

// Request is what one run needs. Templates are absolute paths returned by Verify.
type Request struct {
	Targets   []Target
	Templates []string
	// Timeout is the whole run's time limit (nuclei's own -mt), so a hung program ends.
	Timeout time.Duration
}

// Args builds the argument list or refuses the request. The addresses must be private IPv4 (the scan scope's own rule, checked
// again here), the ports real, the template paths absolute, clean YAML files with no comma (a comma would make nuclei read several).
func Args(r Request) ([]string, error) {
	if err := check(r); err != nil {
		return nil, err
	}
	args := slices.Clone(fixedFlags)
	args = append(args, "-mt", r.Timeout.String())
	for _, t := range r.Targets {
		args = append(args, "-u", t.URL())
	}
	for _, p := range r.Templates {
		args = append(args, "-t", p)
	}
	return args, nil
}

func check(r Request) error {
	switch {
	case len(r.Targets) == 0 || len(r.Targets) > MaxTargets:
		return fmt.Errorf("nuclei: %d targets, want 1 to %d", len(r.Targets), MaxTargets)
	case len(r.Templates) == 0 || len(r.Templates) > MaxTemplates:
		return fmt.Errorf("nuclei: %d templates, want 1 to %d", len(r.Templates), MaxTemplates)
	case r.Timeout <= 0 || r.Timeout > maxRunTime:
		return fmt.Errorf("nuclei: a run needs a time limit of at most %s", maxRunTime)
	}
	if err := checkTargets(r.Targets); err != nil {
		return err
	}
	return checkTemplatePaths(r.Templates)
}

func checkTargets(targets []Target) error {
	for _, t := range targets {
		if !t.Addr.Is4() || !t.Addr.IsPrivate() || t.Port < 1 || t.Port > 65535 {
			return fmt.Errorf("nuclei: %v:%d is not a private IPv4 address and a port", t.Addr, t.Port)
		}
	}
	return nil
}

func checkTemplatePaths(paths []string) error {
	for _, p := range paths {
		if !filepath.IsAbs(p) || filepath.Clean(p) != p || strings.ContainsAny(p, ",\n\x00") || filepath.Ext(p) != ".yaml" {
			return fmt.Errorf("nuclei: %q is not a template path", p)
		}
	}
	return nil
}

// Template is one reviewed template: its id, where it is under the template folder, the SHA-256 of the file as reviewed, and the
// title this repository shows for a hit (the template's own text is never shown).
type Template struct {
	ID     string
	Path   string
	SHA256 string
	Title  string
}

// Verify checks every listed template against its checksum and returns the absolute paths to run. A template that is missing, is
// not a plain file (a link is refused), is outside the folder or differs from what was reviewed makes the whole call fail, naming it.
func Verify(root string, list []Template) ([]string, error) {
	if len(list) == 0 {
		return nil, errors.New("nuclei: no reviewed templates to run")
	}
	paths := make([]string, 0, len(list))
	for _, t := range list {
		p, err := verifyOne(root, t)
		if err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, nil
}

func verifyOne(root string, t Template) (string, error) {
	clean := filepath.Clean(t.Path)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("nuclei: template %s is outside the template folder", t.ID)
	}
	p := filepath.Join(root, clean)
	info, err := os.Lstat(p)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("nuclei: template %s is missing or not a plain file", t.ID)
	}
	f, err := os.Open(p)
	if err != nil {
		return "", fmt.Errorf("nuclei: template %s could not be read: %w", t.ID, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("nuclei: template %s could not be read: %w", t.ID, err)
	}
	if hex.EncodeToString(h.Sum(nil)) != strings.ToLower(t.SHA256) {
		return "", fmt.Errorf("nuclei: template %s is not the file that was reviewed", t.ID)
	}
	return p, nil
}

// Hit is a finding about one target: which reviewed template matched, where, how serious the template says it is (from a fixed set)
// and the title this repository gives that template.
type Hit struct {
	TemplateID string
	Addr       netip.Addr
	Port       int
	Severity   string
	Title      string
}

var severities = []string{"info", "low", "medium", "high", "critical"}

type resultLine struct {
	TemplateID string `json:"template-id"`
	Info       struct {
		Severity string `json:"severity"`
	} `json:"info"`
	Host    string `json:"host"`
	Port    string `json:"port"`
	Matched bool   `json:"matcher-status"`
}

// ParseHits reads the program's JSON lines and keeps a hit only when the template is on the reviewed list, the host and port are one
// that was asked about, and the line says it matched. Every other word of a line is dropped, the template's own text included, so a
// device (or a template) cannot put text in front of the operator or a model. One hit per template and target.
func ParseHits(out []byte, allowed map[string]Template, asked []Target) []Hit {
	var hits []Hit
	seen := map[string]bool{}
	r := bufio.NewReaderSize(bytes.NewReader(out), maxLine)
	for {
		line, tooLong, err := readLine(r)
		if len(line) > 0 && !tooLong {
			h, ok := hitOf(line, allowed, asked)
			key := h.TemplateID + "|" + netip.AddrPortFrom(h.Addr, uint16(h.Port)).String()
			if ok && !seen[key] {
				seen[key] = true
				hits = append(hits, h)
			}
		}
		if err != nil {
			break
		}
	}
	return hits
}

// readLine reads one line of at most maxLine bytes; a longer one is read to its end and reported as too long, so the next line is
// still read.
func readLine(r *bufio.Reader) (line []byte, tooLong bool, err error) {
	line, err = r.ReadSlice('\n')
	for errors.Is(err, bufio.ErrBufferFull) {
		tooLong = true
		_, err = r.ReadSlice('\n')
	}
	return line, tooLong, err
}

func hitOf(line []byte, allowed map[string]Template, asked []Target) (Hit, bool) {
	var l resultLine
	if json.Unmarshal(line, &l) != nil || !l.Matched {
		return Hit{}, false
	}
	tpl, ok := allowed[l.TemplateID]
	if !ok {
		return Hit{}, false
	}
	addr, err := netip.ParseAddr(l.Host)
	port, perr := strconv.Atoi(l.Port)
	if err != nil || perr != nil || !slices.ContainsFunc(asked, func(t Target) bool { return t.Addr == addr && t.Port == port }) {
		return Hit{}, false
	}
	sev := l.Info.Severity
	if !slices.Contains(severities, sev) {
		sev = "info"
	}
	return Hit{TemplateID: tpl.ID, Addr: addr, Port: port, Severity: sev, Title: tpl.Title}, true
}
