package dnscheck

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"llm-proxy/internal/core/tools"
)

type fakeOpener struct {
	body   []byte
	err    error
	target string
	accept string
	calls  int
}

func (f *fakeOpener) OpenAccepting(_ context.Context, target, accept string) (*http.Response, error) {
	f.calls++
	f.target, f.accept = target, accept
	if f.err != nil {
		return nil, f.err
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(f.body)))}, nil
}

func TestOutside_AsksTheSetResolverForTheJSONFormatAndReadsTheReply(t *testing.T) {
	f := &fakeOpener{body: fixture(t, "doh_dns.google.json")}
	o := Outside{Opener: f, URL: func() string { return "https://resolver.example.org/q" }}
	got, err := o.LookupA(context.Background(), "dns.google")
	if err != nil || !Compare(got, addrs(t, "8.8.8.8", "8.8.4.4")).Same {
		t.Fatalf("got %v, %v", got, err)
	}
	if f.target != "https://resolver.example.org/q?name=dns.google&type=A" || f.accept != "application/dns-json" {
		t.Errorf("asked %q accepting %q", f.target, f.accept)
	}
}

func TestOutside_AnEmptySettingAsksTheDefaultResolver(t *testing.T) {
	f := &fakeOpener{body: fixture(t, "doh_dns.google.json")}
	if _, err := (Outside{Opener: f}).LookupA(context.Background(), "dns.google"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(f.target, DefaultResolverURL+"?") {
		t.Errorf("asked %q", f.target)
	}
}

func TestOutside_NeverAsksForABadAddressOrName(t *testing.T) {
	f := &fakeOpener{body: fixture(t, "doh_dns.google.json")}
	bad := []string{"http://resolver.example.org/q", "https://user@resolver.example.org/q", "https://resolver.example.org/q?x=1"}
	for _, base := range bad {
		o := Outside{Opener: f, URL: func() string { return base }}
		if _, err := o.LookupA(context.Background(), "dns.google"); err == nil {
			t.Errorf("%s: want an error", base)
		}
	}
	o := Outside{Opener: f}
	if _, err := o.LookupA(context.Background(), "dns.google&type=TXT#"); err == nil {
		t.Error("a name that is not a host name must never reach the address")
	}
	if f.calls != 0 {
		t.Errorf("nothing may be asked, asked %d times", f.calls)
	}
}

func TestOutside_FailuresAreErrorsAndInternetOffHasItsOwn(t *testing.T) {
	cases := map[string]struct {
		err  error
		body string
		off  bool
	}{
		"internet access off": {err: fmt.Errorf("blocked: %w", tools.ErrInternetBlocked), off: true},
		"network tools off":   {err: tools.ErrNetworkDisabled, off: true},
		"service said 503":    {err: &tools.StatusError{Code: 503, Status: "503"}},
		"connection failed":   {err: errors.New("dial tcp: refused")},
		"not json":            {body: "<html>captive portal</html>"},
	}
	for name, c := range cases {
		o := Outside{Opener: &fakeOpener{err: c.err, body: []byte(c.body)}}
		got, err := o.LookupA(context.Background(), "dns.google")
		if err == nil || got != nil {
			t.Errorf("%s: want an error and no addresses, got %v %v", name, got, err)
		}
		if errors.Is(err, ErrInternetOff) != c.off {
			t.Errorf("%s: internet-off = %v, want %v (%v)", name, errors.Is(err, ErrInternetOff), c.off, err)
		}
	}
}

// An answer is read only up to a bound: a service that pads its reply cannot make the check read all of it.
func TestOutside_ReadsNoMoreThanTheBound(t *testing.T) {
	padded := append([]byte(strings.Repeat(" ", maxBody+1)), fixture(t, "doh_dns.google.json")...)
	o := Outside{Opener: &fakeOpener{body: padded}}
	if got, err := o.LookupA(context.Background(), "dns.google"); err == nil {
		t.Errorf("want an error, read past the bound: %v", got)
	}
}
