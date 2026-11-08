package dnscheck

import (
	"context"
	"errors"
	"strings"
	"testing"

	"llm-proxy-stacks/sdk"
)

type fakeFetch struct {
	body   []byte
	err    error
	target string
	accept string
	calls  int
}

// Get answers as the core's fetch does: at most MaxBytes of the body, with Truncated set when it cut.
func (f *fakeFetch) Get(_ context.Context, req stacksdk.FetchRequest) (stacksdk.FetchResponse, error) {
	f.calls++
	f.target, f.accept = req.URL, req.Accept
	if f.err != nil {
		return stacksdk.FetchResponse{}, f.err
	}
	body, cut := f.body, false
	if req.MaxBytes > 0 && len(body) > req.MaxBytes {
		body, cut = body[:req.MaxBytes], true
	}
	return stacksdk.FetchResponse{Body: body, Truncated: cut}, nil
}

func TestOutside_AsksTheSetResolverForTheJSONFormatAndReadsTheReply(t *testing.T) {
	f := &fakeFetch{body: fixture(t, "doh_dns.google.json")}
	o := Outside{Fetch: f, URL: func() string { return "https://resolver.example.org/q" }}
	got, err := o.LookupA(context.Background(), "dns.google")
	if err != nil || !Compare(got, addrs(t, "8.8.8.8", "8.8.4.4")).Same {
		t.Fatalf("got %v, %v", got, err)
	}
	if f.target != "https://resolver.example.org/q?name=dns.google&type=A" || f.accept != "application/dns-json" {
		t.Errorf("asked %q accepting %q", f.target, f.accept)
	}
}

func TestOutside_AnEmptySettingAsksTheDefaultResolver(t *testing.T) {
	f := &fakeFetch{body: fixture(t, "doh_dns.google.json")}
	if _, err := (Outside{Fetch: f}).LookupA(context.Background(), "dns.google"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(f.target, DefaultResolverURL+"?") {
		t.Errorf("asked %q", f.target)
	}
}

func TestOutside_NeverAsksForABadAddressOrName(t *testing.T) {
	f := &fakeFetch{body: fixture(t, "doh_dns.google.json")}
	bad := []string{"http://resolver.example.org/q", "https://user@resolver.example.org/q", "https://resolver.example.org/q?x=1"}
	for _, base := range bad {
		o := Outside{Fetch: f, URL: func() string { return base }}
		if _, err := o.LookupA(context.Background(), "dns.google"); err == nil {
			t.Errorf("%s: want an error", base)
		}
	}
	o := Outside{Fetch: f}
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
		"internet access off": {err: stacksdk.Errorf(stacksdk.CodeUnavailable, "network or internet access is switched off"), off: true},
		"the guard refused":   {err: stacksdk.Errorf(stacksdk.CodeDenied, "the guard refused this address")},
		"service said 503":    {err: stacksdk.Errorf(stacksdk.CodeInternal, "the server did not answer with HTTP 200")},
		"connection failed":   {err: errors.New("dial tcp: refused")},
		"not json":            {body: "<html>captive portal</html>"},
	}
	for name, c := range cases {
		o := Outside{Fetch: &fakeFetch{err: c.err, body: []byte(c.body)}}
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
	o := Outside{Fetch: &fakeFetch{body: padded}}
	if got, err := o.LookupA(context.Background(), "dns.google"); err == nil {
		t.Errorf("want an error, read past the bound: %v", got)
	}
}
