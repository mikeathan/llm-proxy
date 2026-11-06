package webprobe

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"
)

func allowAll(netip.Addr) error { return nil }

// recorder counts what a test server was asked.
type recorder struct {
	mu      sync.Mutex
	paths   []string
	methods []string
	agents  []string
}

func (r *recorder) note(req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.paths = append(r.paths, req.URL.RequestURI())
	r.methods = append(r.methods, req.Method)
	r.agents = append(r.agents, req.Header.Get("User-Agent"))
}

func (r *recorder) count(path string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(slices.DeleteFunc(slices.Clone(r.paths), func(p string) bool { return p != path }))
}

func targetOf(t *testing.T, srv *httptest.Server, tls bool) Target {
	t.Helper()
	host, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	n, _ := strconv.Atoi(port)
	return Target{Addr: netip.MustParseAddr(host), Port: n, TLS: tls}
}

func prober(guard Guard) *Prober {
	p := New(guard)
	p.pace = 0
	return p
}

func ids(r Result) []string {
	var out []string
	for _, h := range r.Hits {
		out = append(out, h.ID)
	}
	slices.Sort(out)
	return out
}

// device serves the pages of a made-up device: Home Assistant at the root, an nginx status page and a folder file.
func device(rec *recorder) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.note(r)
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte("<title>Home Assistant</title>"))
		case "/nginx_status":
			_, _ = w.Write([]byte("Active connections: 1"))
		case "/.DS_Store":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte("\x00\x00\x00\x01Bud1"))
		default:
			http.NotFound(w, r)
		}
	})
}

func TestProbe_NamesWhatTheDeviceShowsAndNothingElse(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(device(rec))
	defer srv.Close()
	res, err := prober(allowAll).Probe(context.Background(), []Target{targetOf(t, srv, false)}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"ds_store_file", "home_assistant_page", "nginx_status_page"}; !slices.Equal(ids(res), want) {
		t.Errorf("hits = %v, want %v", ids(res), want)
	}
	if res.Partial {
		t.Error("a run that finished is not partial")
	}
	for _, h := range res.Hits {
		if h.Title == "" || h.Severity == "" || h.Port == 0 {
			t.Errorf("hit = %+v", h)
		}
	}
}

func TestProbe_AsksPlainlyOncePerPathAndOnlyWithGet(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(device(rec))
	defer srv.Close()
	if _, err := prober(allowAll).Probe(context.Background(), []Target{targetOf(t, srv, false)}, time.Minute); err != nil {
		t.Fatal(err)
	}
	if n := rec.count("/"); n != 1 {
		t.Errorf("the root was asked %d times, want once for all the checks that read it", n)
	}
	for _, m := range rec.methods {
		if m != http.MethodGet {
			t.Errorf("method %s: only GET may be sent", m)
		}
	}
	for _, a := range rec.agents {
		if a != userAgent {
			t.Errorf("user agent %q", a)
		}
	}
	distinct := map[string]bool{}
	for _, c := range Checks {
		for _, p := range c.Paths {
			distinct[p] = true
		}
	}
	if len(rec.paths) != len(distinct) {
		t.Errorf("%d requests for %d distinct paths", len(rec.paths), len(distinct))
	}
}

func TestProbe_NeverFollowsARedirect(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.note(r)
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/elsewhere", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("<title>Home Assistant</title>"))
	}))
	defer srv.Close()
	res, err := prober(allowAll).Probe(context.Background(), []Target{targetOf(t, srv, false)}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if rec.count("/elsewhere") != 0 {
		t.Error("a redirect was followed")
	}
	if len(res.Hits) != 0 {
		t.Errorf("hits = %v: a redirect answer is not a page", ids(res))
	}
}

func TestProbe_ReadsATLSServiceWithoutTrustingItsCertificate(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewTLSServer(device(rec))
	defer srv.Close()
	res, err := prober(allowAll).Probe(context.Background(), []Target{targetOf(t, srv, true)}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(ids(res), "home_assistant_page") {
		t.Errorf("hits = %v", ids(res))
	}
}

func TestProbe_RefusesATargetOutsideTheScopeBeforeSendingAnything(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(device(rec))
	defer srv.Close()
	deny := func(netip.Addr) error { return errors.New("not in scope") }
	if _, err := prober(deny).Probe(context.Background(), []Target{targetOf(t, srv, false)}, time.Minute); err == nil {
		t.Fatal("want an error")
	}
	if len(rec.paths) != 0 {
		t.Errorf("%d requests were sent to a target the scope refused", len(rec.paths))
	}
}

func TestProbe_TheGuardIsAskedAgainAtEveryConnection(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(device(rec))
	defer srv.Close()
	var mu sync.Mutex
	calls := 0
	guard := func(netip.Addr) error {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if calls > 1 { // the check before the run passes, the connection is refused
			return errors.New("not in scope")
		}
		return nil
	}
	res, err := prober(guard).Probe(context.Background(), []Target{targetOf(t, srv, false)}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.paths) != 0 || len(res.Hits) != 0 {
		t.Errorf("a connection the guard refused still reached the device: %d requests, hits %v", len(rec.paths), ids(res))
	}
}

func TestProbe_ATimeLimitEndsTheRunAndSaysSo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
	}))
	defer srv.Close()
	res, err := prober(allowAll).Probe(context.Background(), []Target{targetOf(t, srv, false)}, 120*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Partial {
		t.Error("a run that hit its limit must say it is partial")
	}
}

func TestProbe_ACancelledRunIsAnErrorNotAPartialResult(t *testing.T) {
	srv := httptest.NewServer(device(&recorder{}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := prober(allowAll).Probe(ctx, []Target{targetOf(t, srv, false)}, time.Minute); err == nil {
		t.Error("want the cancellation")
	}
}

func TestProbe_RefusesAnUnusableRequest(t *testing.T) {
	p := prober(allowAll)
	good := Target{Addr: netip.MustParseAddr("192.168.1.5"), Port: 80}
	for name, run := range map[string]func() error{
		"no targets": func() error { _, err := p.Probe(context.Background(), nil, time.Minute); return err },
		"no limit":   func() error { _, err := p.Probe(context.Background(), []Target{good}, 0); return err },
		"ipv6": func() error {
			_, err := p.Probe(context.Background(), []Target{{Addr: netip.MustParseAddr("fe80::1"), Port: 80}}, time.Minute)
			return err
		},
		"port zero": func() error {
			_, err := p.Probe(context.Background(), []Target{{Addr: good.Addr}}, time.Minute)
			return err
		},
		"too many": func() error {
			_, err := p.Probe(context.Background(), slices.Repeat([]Target{good}, MaxTargets+1), time.Minute)
			return err
		},
	} {
		if run() == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestProbe_ADeviceThatDoesNotAnswerIsNotAnError(t *testing.T) {
	srv := httptest.NewServer(device(&recorder{}))
	target := targetOf(t, srv, false)
	srv.Close()
	res, err := prober(allowAll).Probe(context.Background(), []Target{target}, time.Minute)
	if err != nil || len(res.Hits) != 0 || res.Partial {
		t.Errorf("res = %+v, err = %v", res, err)
	}
}

func TestProbe_ReadsOnlyTheStartOfALongAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		pad := make([]byte, maxBody)
		for i := range pad {
			pad[i] = ' '
		}
		_, _ = w.Write(pad)
		_, _ = w.Write([]byte("<title>Home Assistant</title>")) // beyond the cap: never looked at
	}))
	defer srv.Close()
	res, err := prober(allowAll).Probe(context.Background(), []Target{targetOf(t, srv, false)}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) != 0 {
		t.Errorf("hits = %v: text past the cap was read", ids(res))
	}
}
