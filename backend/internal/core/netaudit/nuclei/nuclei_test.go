package nuclei

import (
	"crypto/sha256"
	"encoding/hex"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func targets(t ...Target) []Target { return t }

func tgt(addr string, port int, tls bool) Target {
	return Target{Addr: mustAddr(addr), Port: port, TLS: tls}
}

// The flags were read from `nuclei -h` of v3.11.1 (verified 2026-11-05). This table is the only way a flag reaches the program.
func TestArgs_AreTheFixedTableTheTargetsAndTheTemplatesAndNothingElse(t *testing.T) {
	args, err := Args(Request{Targets: targets(tgt("192.168.1.20", 8080, false), tgt("192.168.1.30", 443, true)), Templates: []string{"/t/a.yaml", "/t/b.yaml"}, Timeout: 90 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"-duc", "-ni", "-nc", "-silent", "-j", "-or", "-ot", "-dr", "-nmhe"} {
		if !slices.Contains(args, want) {
			t.Errorf("missing %s in %v", want, args)
		}
	}
	if i := slices.Index(args, "-pt"); i < 0 || args[i+1] != "http" {
		t.Errorf("only http templates may run: %v", args)
	}
	if i := slices.Index(args, "-mt"); i < 0 || args[i+1] != "1m30s" {
		t.Errorf("the run must have a time limit: %v", args)
	}
	for _, flag := range []string{"-rl", "-c", "-bs", "-timeout", "-retries", "-rsr"} {
		if !slices.Contains(args, flag) {
			t.Errorf("missing the %s bound", flag)
		}
	}
	for _, banned := range []string{"-ut", "-update-templates", "-headless", "-code", "-file", "-dast", "-esc", "-ud", "-uc", "-config", "-l", "-list", "-w", "-workflows", "-tags", "-severity", "-id", "-tid"} {
		if slices.Contains(args, banned) {
			t.Errorf("flag %s must never be passed", banned)
		}
	}
	var urls, files []string
	for i, a := range args {
		switch a {
		case "-u":
			urls = append(urls, args[i+1])
		case "-t":
			files = append(files, args[i+1])
		}
	}
	if !slices.Equal(urls, []string{"http://192.168.1.20:8080", "https://192.168.1.30:443"}) {
		t.Errorf("targets = %v", urls)
	}
	if !slices.Equal(files, []string{"/t/a.yaml", "/t/b.yaml"}) {
		t.Errorf("templates = %v", files)
	}
}

func TestArgs_RefuseWhatTheScopeRulesWouldNot(t *testing.T) {
	ok := Request{Targets: targets(tgt("192.168.1.20", 80, false)), Templates: []string{"/t/a.yaml"}, Timeout: time.Minute}
	cases := map[string]func(*Request){
		"no target":          func(r *Request) { r.Targets = nil },
		"no template":        func(r *Request) { r.Templates = nil },
		"a public address":   func(r *Request) { r.Targets = targets(tgt("8.8.8.8", 80, false)) },
		"loopback":           func(r *Request) { r.Targets = targets(tgt("127.0.0.1", 80, false)) },
		"link local":         func(r *Request) { r.Targets = targets(tgt("169.254.1.1", 80, false)) },
		"an IPv6 address":    func(r *Request) { r.Targets = targets(tgt("fe80::1", 80, false)) },
		"port zero":          func(r *Request) { r.Targets = targets(tgt("192.168.1.20", 0, false)) },
		"a huge port":        func(r *Request) { r.Targets = targets(tgt("192.168.1.20", 70000, false)) },
		"a relative path":    func(r *Request) { r.Templates = []string{"a.yaml"} },
		"a flag as a path":   func(r *Request) { r.Templates = []string{"-ut"} },
		"a comma in a path":  func(r *Request) { r.Templates = []string{"/t/a.yaml,/etc/passwd"} },
		"a non yaml file":    func(r *Request) { r.Templates = []string{"/t/a.sh"} },
		"a path with ..":     func(r *Request) { r.Templates = []string{"/t/../a.yaml"} },
		"too many targets":   func(r *Request) { r.Targets = slices.Repeat(targets(tgt("192.168.1.20", 80, false)), MaxTargets+1) },
		"too many templates": func(r *Request) { r.Templates = slices.Repeat([]string{"/t/a.yaml"}, MaxTemplates+1) },
		"no time limit":      func(r *Request) { r.Timeout = 0 },
		"a day long run":     func(r *Request) { r.Timeout = 24 * time.Hour },
	}
	for name, mutate := range cases {
		r := ok
		mutate(&r)
		if args, err := Args(r); err == nil {
			t.Errorf("%s: accepted, args = %v", name, args)
		}
	}
	if _, err := Args(ok); err != nil {
		t.Fatalf("the baseline request must be accepted: %v", err)
	}
}

func writeTemplate(t *testing.T, dir, rel, body string) Template {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(body))
	return Template{ID: strings.TrimSuffix(filepath.Base(rel), ".yaml"), Path: rel, SHA256: hex.EncodeToString(sum[:]), Title: "A title"}
}

func TestVerify_ReturnsThePathsOfTemplatesThatAreExactlyWhatWasReviewed(t *testing.T) {
	dir := t.TempDir()
	a := writeTemplate(t, dir, "http/a.yaml", "id: a\n")
	b := writeTemplate(t, dir, "http/sub/b.yaml", "id: b\n")
	paths, err := Verify(dir, []Template{a, b})
	if err != nil || !slices.Equal(paths, []string{filepath.Join(dir, "http/a.yaml"), filepath.Join(dir, "http/sub/b.yaml")}) {
		t.Fatalf("paths = %v err = %v", paths, err)
	}
}

func TestVerify_RefusesAChangedMissingOrEscapingTemplateAndNamesIt(t *testing.T) {
	dir := t.TempDir()
	a := writeTemplate(t, dir, "http/a.yaml", "id: a\n")
	if err := os.WriteFile(filepath.Join(dir, "http/a.yaml"), []byte("id: a\nhttp: [evil]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(dir, []Template{a}); err == nil || !strings.Contains(err.Error(), "a") {
		t.Fatalf("a changed template must be refused and named: %v", err)
	}
	missing := Template{ID: "gone", Path: "http/gone.yaml", SHA256: strings.Repeat("0", 64)}
	if _, err := Verify(dir, []Template{missing}); err == nil {
		t.Fatal("a missing template must be refused")
	}
	escape := Template{ID: "esc", Path: "../outside.yaml", SHA256: strings.Repeat("0", 64)}
	if _, err := Verify(dir, []Template{escape}); err == nil {
		t.Fatal("a path outside the template folder must be refused")
	}
	if _, err := Verify(dir, nil); err == nil {
		t.Fatal("an empty list must be refused: there would be nothing reviewed to run")
	}
}

func TestVerify_RefusesALinkSoAReviewedNameCannotPointElsewhere(t *testing.T) {
	dir := t.TempDir()
	good := writeTemplate(t, dir, "http/real.yaml", "id: real\n")
	if err := os.Symlink(filepath.Join(dir, "http/real.yaml"), filepath.Join(dir, "http/link.yaml")); err != nil {
		t.Skip("no symlinks here")
	}
	link := good
	link.ID, link.Path = "link", "http/link.yaml"
	if _, err := Verify(dir, []Template{link}); err == nil {
		t.Fatal("a symbolic link must be refused even when its target matches")
	}
}

func list() map[string]Template {
	return map[string]Template{"ds-store-file": {ID: "ds-store-file", Title: "A .DS_Store file is exposed"}}
}

func TestParseHits_ReadsARealResultLineAndKeepsOnlyTypedFacts(t *testing.T) {
	raw, err := os.ReadFile("testdata/ds_store_hit.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	hits := ParseHits(raw, list(), targets(tgt("192.168.1.20", 8080, false)))
	if len(hits) != 1 {
		t.Fatalf("hits = %+v", hits)
	}
	h := hits[0]
	if h.TemplateID != "ds-store-file" || h.Addr != mustAddr("192.168.1.20") || h.Port != 8080 || h.Severity != "low" || h.Title != "A .DS_Store file is exposed" {
		t.Fatalf("hit = %+v; want our own title, not the template's text", h)
	}
}

func TestParseHits_IgnoresEverythingThatIsNotAnAnswerAboutARequestedTargetFromAListedTemplate(t *testing.T) {
	good := `{"template-id":"ds-store-file","info":{"severity":"low"},"host":"192.168.1.20","port":"8080","matcher-status":true}`
	for name, line := range map[string]string{
		"a template not on the list": strings.Replace(good, "ds-store-file", "other", 1),
		"a host nobody asked about":  strings.Replace(good, "192.168.1.20", "192.168.1.99", 1),
		"a port nobody asked about":  strings.Replace(good, "8080", "9090", 1),
		"not a match":                strings.Replace(good, "true", "false", 1),
		"not json":                   "ds-store-file 192.168.1.20 8080",
		"a name for a host":          strings.Replace(good, "192.168.1.20", "evil.example.com", 1),
		"a port as text":             strings.Replace(good, `"8080"`, `"80;rm"`, 1),
	} {
		if hits := ParseHits([]byte(line+"\n"), list(), targets(tgt("192.168.1.20", 8080, false))); len(hits) != 0 {
			t.Errorf("%s: kept %+v", name, hits)
		}
	}
}

func TestParseHits_ASeverityOutsideTheFixedSetIsInfoAndOneLineIsBounded(t *testing.T) {
	line := `{"template-id":"ds-store-file","info":{"severity":"<b>critical</b>"},"host":"192.168.1.20","port":"8080","matcher-status":true}`
	hits := ParseHits([]byte(line+"\n"), list(), targets(tgt("192.168.1.20", 8080, false)))
	if len(hits) != 1 || hits[0].Severity != "info" {
		t.Fatalf("hits = %+v", hits)
	}
	huge := `{"template-id":"ds-store-file","host":"192.168.1.20","port":"8080","matcher-status":true,"x":"` + strings.Repeat("a", maxLine) + `"}`
	if hits := ParseHits([]byte(huge+"\n"+line+"\n"), list(), targets(tgt("192.168.1.20", 8080, false))); len(hits) != 1 {
		t.Fatalf("an over-long line must be skipped and the next one read: %+v", hits)
	}
}

func TestParseHits_OneHitPerTemplateAndTarget(t *testing.T) {
	line := `{"template-id":"ds-store-file","info":{"severity":"low"},"host":"192.168.1.20","port":"8080","matcher-status":true}`
	hits := ParseHits([]byte(line+"\n"+line+"\n"), list(), targets(tgt("192.168.1.20", 8080, false)))
	if len(hits) != 1 {
		t.Fatalf("hits = %+v, want the repeat dropped", hits)
	}
}

func mustAddr(s string) netip.Addr { return netip.MustParseAddr(s) }
