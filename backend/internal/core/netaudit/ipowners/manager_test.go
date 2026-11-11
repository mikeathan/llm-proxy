package ipowners

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"
)

func gz(t *testing.T, text string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	_, _ = w.Write([]byte(text))
	_ = w.Close()
	return b.Bytes()
}

type memStore struct {
	data   []byte
	at     time.Time
	writes int
	err    error
}

func (s *memStore) Read() ([]byte, time.Time, error) { return s.data, s.at, s.err }
func (s *memStore) Write(b []byte, at time.Time) error {
	if s.err != nil {
		return s.err
	}
	s.data, s.at = b, at
	s.writes++
	return nil
}
func (s *memStore) Delete() error { s.data, s.at = nil, time.Time{}; return nil }

// feed answers each address the manager asks for, and records which.
type feed struct {
	answers map[string]func() (*http.Response, error)
	asked   []string
}

func (f *feed) open(_ context.Context, url string) (*http.Response, error) {
	f.asked = append(f.asked, url)
	if fn, ok := f.answers[url]; ok {
		return fn()
	}
	return nil, errors.New("not expected")
}

func ok(body []byte) func() (*http.Response, error) {
	return func() (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body))}, nil
	}
}

func newFeed(t *testing.T) *feed {
	t.Helper()
	v4, v6 := "", ""
	for _, line := range strings.SplitAfter(sample, "\n") {
		if strings.Contains(line, ":") {
			v6 += line
		} else {
			v4 += line
		}
	}
	return &feed{answers: map[string]func() (*http.Response, error){UrlV4: ok(gz(t, v4)), UrlV6: ok(gz(t, v6))}}
}

func newManager(t *testing.T, st *memStore, f *feed) *Manager {
	t.Helper()
	m, err := NewManager(Deps{Store: st, Open: f.open, Now: func() time.Time { return time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC) }})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestNothingIsKnownUntilTheTableIsDownloaded(t *testing.T) {
	m := newManager(t, &memStore{}, newFeed(t))
	if _, ok := m.Owner(netip.MustParseAddr("8.8.8.8")); ok {
		t.Error("an address had an owner before any table was downloaded")
	}
	if s := m.Status(); s.Ranges != 0 || !s.LoadedAt.IsZero() {
		t.Errorf("status = %+v, want nothing held", s)
	}
}

func TestDownloadFetchesOnlyTheTwoFixedAddressesAndHoldsTheTable(t *testing.T) {
	f, st := newFeed(t), &memStore{}
	m := newManager(t, st, f)
	if err := m.Download(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.asked) != 2 || f.asked[0] != UrlV4 || f.asked[1] != UrlV6 {
		t.Errorf("asked %v, want exactly the two fixed addresses", f.asked)
	}
	if o, ok := m.Owner(netip.MustParseAddr("8.8.8.8")); !ok || o.ASN != 15169 {
		t.Errorf("owner = %+v %v", o, ok)
	}
	if s := m.Status(); s.Ranges != 5 || s.LoadedAt.IsZero() {
		t.Errorf("status = %+v, want 5 ranges and a time", s)
	}
	if st.writes != 1 || len(st.data) == 0 {
		t.Errorf("the table was not kept: %d writes, %d bytes", st.writes, len(st.data))
	}
}

func TestAHeldTableComesBackWhenTheProgramRestarts(t *testing.T) {
	f, st := newFeed(t), &memStore{}
	if err := newManager(t, st, f).Download(context.Background()); err != nil {
		t.Fatal(err)
	}
	again := newManager(t, st, &feed{})
	if o, ok := again.Owner(netip.MustParseAddr("2606:4700::1")); !ok || o.ASN != 13335 {
		t.Errorf("owner after a restart = %+v %v", o, ok)
	}
	if again.Status().LoadedAt.IsZero() {
		t.Error("the time of the download was lost")
	}
}

func TestADownloadThatFailsKeepsTheTableThatWasHeld(t *testing.T) {
	f, st := newFeed(t), &memStore{}
	m := newManager(t, st, f)
	if err := m.Download(context.Background()); err != nil {
		t.Fatal(err)
	}
	for name, bad := range map[string]func() (*http.Response, error){
		"a refusal": func() (*http.Response, error) {
			return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader(""))}, nil
		},
		"a network error": func() (*http.Response, error) { return nil, errors.New("boom") },
		"not gzip":        ok([]byte("this is not gzip")),
		"an empty table":  ok(gz(t, "# nothing\n")),
	} {
		f.answers[UrlV6] = bad
		if err := m.Download(context.Background()); err == nil {
			t.Errorf("%s: no error", name)
		}
		if o, ok := m.Owner(netip.MustParseAddr("8.8.8.8")); !ok || o.ASN != 15169 {
			t.Errorf("%s: the held table was lost: %+v %v", name, o, ok)
		}
	}
	if st.writes != 1 {
		t.Errorf("%d writes: a failed download replaced the stored file", st.writes)
	}
}

func TestABodyThatExpandsPastTheLimitIsRefusedNotCut(t *testing.T) {
	f, st := newFeed(t), &memStore{}
	was := maxExpanded
	maxExpanded = 200
	defer func() { maxExpanded = was }()
	f.answers[UrlV4] = ok(gz(t, strings.Repeat("1.0.0.0\t1.0.0.255\t1\tUS\tx\n", 100)))
	m := newManager(t, st, f)
	err := m.Download(context.Background())
	if err == nil || !strings.Contains(err.Error(), "larger") {
		t.Fatalf("err = %v, want a refusal saying it is too large", err)
	}
	if st.writes != 0 {
		t.Error("a refused download was stored")
	}
}

func TestADownloadErrorNamesTheServiceAndNoAddressOfTheOperators(t *testing.T) {
	f := newFeed(t)
	f.answers[UrlV4] = func() (*http.Response, error) {
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader(""))}, nil
	}
	err := newManager(t, &memStore{}, f).Download(context.Background())
	if err == nil || !strings.Contains(err.Error(), "iptoasn.com") || !strings.Contains(err.Error(), "404") {
		t.Errorf("err = %v", err)
	}
}

func TestRemoveForgetsTheTable(t *testing.T) {
	f, st := newFeed(t), &memStore{}
	m := newManager(t, st, f)
	if err := m.Download(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove(); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Owner(netip.MustParseAddr("8.8.8.8")); ok || m.Status().Ranges != 0 || st.data != nil {
		t.Error("the table was still there after Remove")
	}
}

func TestAStoredFileThatCannotBeReadStartsTheManagerEmptyAndSaysSo(t *testing.T) {
	m, err := NewManager(Deps{Store: &memStore{data: []byte("garbage"), at: time.Now()}, Open: (&feed{}).open})
	if err == nil || m == nil {
		t.Fatalf("err = %v, manager = %v; want a usable empty manager and the error to log", err, m)
	}
	if _, ok := m.Owner(netip.MustParseAddr("8.8.8.8")); ok {
		t.Error("a corrupt file gave an owner")
	}
}

// A real download of the public table, to measure what the bounds must hold and what a lookup costs. It runs only when asked, because it goes out
// to the internet: IPOWNERS_LIVE=1 go test ./internal/core/netaudit/ipowners/ -run Live -v
func TestLiveTheRealTableFitsTheBoundsAndAnswers(t *testing.T) {
	if testing.Short() || os.Getenv("IPOWNERS_LIVE") == "" {
		t.Skip("set IPOWNERS_LIVE=1 to download the real table")
	}
	open := func(ctx context.Context, url string) (*http.Response, error) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		return http.DefaultClient.Do(req)
	}
	st := &memStore{}
	start := time.Now()
	m, _ := NewManager(Deps{Store: st, Open: open})
	if err := m.Download(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Logf("downloaded and parsed in %s: %+v, %d bytes stored", time.Since(start), m.Status(), len(st.data))
	start = time.Now()
	again, err := NewManager(Deps{Store: st, Open: open})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("loaded from the stored file in %s: %+v", time.Since(start), again.Status())
	for _, a := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111", "2001:4860:4860::8888"} {
		o, ok := again.Owner(netip.MustParseAddr(a))
		t.Logf("%s -> %+v %v", a, o, ok)
		if !ok {
			t.Errorf("%s has no owner", a)
		}
	}
}
