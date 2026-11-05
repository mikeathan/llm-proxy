package knownbad

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"llm-proxy/models"
)

func sample(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/hostfile_sample.txt")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestParse_ReadsTheRealHostFileShapeAndKeepsOnlyPlainNames(t *testing.T) {
	names, err := Parse(strings.NewReader(sample(t)))
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 10 {
		t.Fatalf("names = %d, want the 10 entries of the sample", len(names))
	}
	for _, want := range []string{"123.ywxww.net", "a.g6.wtf", "434f06c-geik4j16cf-d1fa-73d9f.acutelyunfailing.cc"} {
		if !contains(names, want) {
			t.Errorf("missing %s", want)
		}
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestParse_AcceptsBareNamesAndZeroAddressesAndDropsEverythingElse(t *testing.T) {
	in := strings.Join([]string{
		"# a comment", "", "0.0.0.0 Bare.EXAMPLE", "127.0.0.1\tgood.example.org", "only.example.net",
		"127.0.0.1 bad name.example", "127.0.0.1 <script>.example", "8.8.8.8 evil.example", "127.0.0.1 a..b", "127.0.0.1 1.2.3.in-addr.arpa",
		"127.0.0.1 " + strings.Repeat("a", 64) + ".example",
	}, "\n")
	names, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"bare.example", "good.example.org", "only.example.net"}
	if len(names) != len(want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
	for _, w := range want {
		if !contains(names, w) {
			t.Errorf("missing %s in %v", w, names)
		}
	}
}

func TestParse_ARouteToAnotherAddressIsNotAHostFileLine(t *testing.T) {
	names, _ := Parse(strings.NewReader("8.8.8.8 redirected.example\n"))
	if len(names) != 0 {
		t.Fatalf("names = %v; a line that points a name at a real address is not a block entry", names)
	}
}

func TestParse_AnEmptyOrOversizeFileIsRefused(t *testing.T) {
	if _, err := Parse(strings.NewReader("# only comments\n")); err == nil {
		t.Fatal("a file with no names must be refused, not loaded as an empty list that reads as clean")
	}
	var b strings.Builder
	for i := 0; i <= MaxNames; i++ {
		fmt.Fprintf(&b, "127.0.0.1 h%d.example\n", i)
	}
	if _, err := Parse(strings.NewReader(b.String())); err == nil {
		t.Fatal("a list over the bound must be refused")
	}
}

func TestSet_MatchesAListedNameAndAnythingUnderIt(t *testing.T) {
	s := NewSet([]string{"acutelyunfailing.cc", "exact.example.org"})
	for name, want := range map[string]string{
		"acutelyunfailing.cc": "acutelyunfailing.cc", "434f.acutelyunfailing.cc": "acutelyunfailing.cc", "a.b.acutelyunfailing.cc": "acutelyunfailing.cc",
		"exact.example.org": "exact.example.org",
	} {
		if got, ok := s.Match(name); !ok || got != want {
			t.Errorf("Match(%q) = %q %v, want %q", name, got, ok, want)
		}
	}
	for _, name := range []string{"example.org", "notacutelyunfailing.cc", "cc", "", "other.example.org"} {
		if got, ok := s.Match(name); ok {
			t.Errorf("Match(%q) = %q, want no match (a parent or a lookalike is not listed)", name, got)
		}
	}
	if _, ok := NewSet(nil).Match("anything.example"); ok {
		t.Error("an empty set matches nothing")
	}
}

type memStore struct {
	state models.KnownBad
	err   error
}

func (m *memStore) Read() (models.KnownBad, error) { return m.state, m.err }
func (m *memStore) Write(s models.KnownBad) error  { m.state = s; return m.err }

func opener(body string, status int, got *string) Opener {
	return func(_ context.Context, url string) (*http.Response, error) {
		*got = url
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
	}
}

const goodURL = "https://urlhaus-api.abuse.ch/v2/files/exports/0123456789abcdef/hostfile.txt"

func newManager(t *testing.T, open Opener, secret string) (*Manager, *memStore) {
	t.Helper()
	st := &memStore{}
	now := time.Date(2026, 11, 5, 16, 0, 0, 0, time.UTC)
	m, err := NewManager(Deps{Store: st, Open: open, Address: func() string { return secret }, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	return m, st
}

func TestManager_DownloadLoadsTheListAndKeepsItAcrossARestart(t *testing.T) {
	var asked string
	m, st := newManager(t, opener(sample(t), 200, &asked), goodURL)
	if err := m.Download(context.Background()); err != nil {
		t.Fatal(err)
	}
	if asked != goodURL {
		t.Fatalf("asked %q", asked)
	}
	if got, ok := m.Match("sub.acutelyunfailing.cc"); ok || got != "" {
		t.Fatalf("the sample lists 434f….acutelyunfailing.cc, not its parent: %q %v", got, ok)
	}
	if got, ok := m.Match("a.g6.wtf"); !ok || got != "a.g6.wtf" {
		t.Fatalf("Match = %q %v", got, ok)
	}
	if st := m.Status(); st.Count != 10 || st.LoadedAt.IsZero() {
		t.Fatalf("status = %+v", st)
	}
	again, err := NewManager(Deps{Store: st, Open: opener("", 500, &asked), Address: func() string { return "" }, Now: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := again.Match("a.g6.wtf"); !ok {
		t.Fatal("the list must survive a restart")
	}
}

func TestManager_RefusesWhatIsNotTheListAndKeepsTheOldOne(t *testing.T) {
	var asked string
	m, _ := newManager(t, opener(sample(t), 200, &asked), goodURL)
	if err := m.Download(context.Background()); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		open   Opener
		secret string
	}{
		"no address":        {opener(sample(t), 200, &asked), ""},
		"plain http":        {opener(sample(t), 200, &asked), "http://urlhaus-api.abuse.ch/x"},
		"another host":      {opener(sample(t), 200, &asked), "https://evil.example.com/hostfile.txt"},
		"a lookalike host":  {opener(sample(t), 200, &asked), "https://abuse.ch.evil.example/hostfile.txt"},
		"a refused request": {opener("denied", 403, &asked), goodURL},
		"an empty answer":   {opener("# nothing\n", 200, &asked), goodURL},
	} {
		m.deps.Address = func() string { return c.secret }
		m.deps.Open = c.open
		if err := m.Download(context.Background()); err == nil {
			t.Errorf("%s: accepted", name)
		}
		if _, ok := m.Match("a.g6.wtf"); !ok {
			t.Fatalf("%s: a failed download must keep the list that was held", name)
		}
	}
	m.deps.Address, m.deps.Open = func() string { return goodURL }, func(context.Context, string) (*http.Response, error) { return nil, errors.New("offline") }
	if err := m.Download(context.Background()); err == nil || strings.Contains(err.Error(), "0123456789abcdef") {
		t.Fatalf("err = %v: an error must never repeat the address, which holds the key", err)
	}
}

func TestValidAddress_AcceptsOnlyAnHTTPSAddressOnAbuseCh(t *testing.T) {
	for addr, want := range map[string]bool{
		goodURL: true, "https://urlhaus.abuse.ch/x": true, "https://abuse.ch/x": true,
		"http://urlhaus-api.abuse.ch/x": false, "https://evil.example/x": false, "https://abuse.ch.evil.example/x": false,
		"": false, "not a url": false, "https://user:pw@urlhaus-api.abuse.ch/x": false,
	} {
		if got := ValidAddress(addr) == nil; got != want {
			t.Errorf("ValidAddress(%q) ok = %v, want %v", addr, got, want)
		}
	}
}
