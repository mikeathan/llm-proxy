package outsidelookup

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"llm-proxy/internal/core/tools"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// opener answers every request with one canned reply and remembers the address it was asked for.
type opener struct {
	status int
	body   string
	err    error
	asked  string
}

func (o *opener) Open(_ context.Context, target string) (*http.Response, error) {
	o.asked = target
	if o.err != nil {
		return nil, o.err
	}
	if o.status != http.StatusOK {
		return nil, &tools.StatusError{Code: o.status, Status: http.StatusText(o.status)}
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(o.body))}, nil
}

func clientOf(o *opener, base string) Client {
	return Client{Opener: o, BaseURL: func() string { return base }}
}

var public = netip.MustParseAddr("203.0.113.7")

// internetdb_ports.json and internetdb_no_data.json are real replies of the service (2026-11-01); the address in the first is
// replaced by a documentation address.
func TestOpenPorts_TheRealReplyGivesItsPorts(t *testing.T) {
	o := &opener{status: 200, body: fixture(t, "internetdb_ports.json")}
	got, err := clientOf(o, DefaultURL).OpenPorts(context.Background(), public)
	if err != nil || !reflect.DeepEqual(got, []int{1194}) {
		t.Errorf("got %v, %v", got, err)
	}
	if o.asked != "https://internetdb.shodan.io/203.0.113.7" {
		t.Errorf("asked %q", o.asked)
	}
}

func TestOpenPorts_TheRealNoDataReplyMeansNoPortsKnown(t *testing.T) {
	// The service answers 404 with {"detail":"No information available"} for an address it has nothing on.
	got, err := clientOf(&opener{status: 404, body: fixture(t, "internetdb_no_data.json")}, DefaultURL).OpenPorts(context.Background(), public)
	if err != nil || len(got) != 0 {
		t.Errorf("got %v, %v", got, err)
	}
}

func TestOpenPorts_OtherFailuresAreErrorsNeverEmptyLists(t *testing.T) {
	for name, o := range map[string]*opener{
		"server error": {status: 500},
		"rate limited": {status: 429},
		"blocked":      {err: errors.New("network access is fully blocked by guardrails")},
		"not json":     {status: 200, body: "<html>"},
		"no ports key": {status: 200, body: `{"ip":"203.0.113.7"}`},
	} {
		if got, err := clientOf(o, DefaultURL).OpenPorts(context.Background(), public); err == nil {
			t.Errorf("%s must be an error, got %v", name, got)
		}
	}
}

func TestOpenPorts_KeepsOnlyValidPortsOnceEach(t *testing.T) {
	o := &opener{status: 200, body: `{"ports":[443,0,70000,-1,443,22,"x"],"vulns":["CVE-1"],"hostnames":["a.example"]}`}
	got, err := clientOf(o, DefaultURL).OpenPorts(context.Background(), public)
	if err == nil && !reflect.DeepEqual(got, []int{22, 443}) {
		t.Errorf("got %v", got)
	}
	if err != nil {
		t.Errorf("a stray entry must be skipped, not fail the read: %v", err)
	}
}

func TestOpenPorts_IsBounded(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"ports":[`)
	for i := 1; i <= 1000; i++ {
		if i > 1 {
			b.WriteString(",")
		}
		b.WriteString(strconv.Itoa(i))
	}
	b.WriteString(`]}`)
	got, err := clientOf(&opener{status: 200, body: b.String()}, DefaultURL).OpenPorts(context.Background(), public)
	if err != nil || len(got) != MaxPorts {
		t.Errorf("len %d, %v", len(got), err)
	}
}

func TestOpenPorts_OnlyAPublicAddressIsEverSent(t *testing.T) {
	for _, a := range []string{"192.168.0.1", "100.64.0.1", "127.0.0.1", "169.254.1.1", "0.0.0.0"} {
		o := &opener{status: 200, body: `{"ports":[]}`}
		if _, err := clientOf(o, DefaultURL).OpenPorts(context.Background(), netip.MustParseAddr(a)); err == nil || o.asked != "" {
			t.Errorf("%s must not be sent (asked %q, err %v)", a, o.asked, err)
		}
	}
}

func TestValidateBaseURL(t *testing.T) {
	good := map[string]string{
		"https://internetdb.shodan.io":       "https://internetdb.shodan.io",
		"https://internetdb.shodan.io/":      "https://internetdb.shodan.io",
		"https://lookup.example.org:8443/db": "https://lookup.example.org:8443/db",
	}
	for in, want := range good {
		if got, err := ValidateBaseURL(in); err != nil || got != want {
			t.Errorf("%q: got %q, %v", in, got, err)
		}
	}
	for _, in := range []string{"", "http://internetdb.shodan.io", "ftp://x.example", "https://", "https://u:p@x.example", "https://x.example?q=1", "https://x.example#f", "internetdb.shodan.io", "https://x.example/a b"} {
		if got, err := ValidateBaseURL(in); err == nil {
			t.Errorf("%q must be refused, got %q", in, got)
		}
	}
}

func TestOpenPorts_AnEmptyOrBadConfiguredURLUsesNothing(t *testing.T) {
	o := &opener{status: 200, body: `{"ports":[]}`}
	if _, err := clientOf(o, "http://insecure.example").OpenPorts(context.Background(), public); err == nil || o.asked != "" {
		t.Errorf("a plain-http service must never be asked (asked %q, %v)", o.asked, err)
	}
	if _, err := clientOf(o, "").OpenPorts(context.Background(), public); err != nil || o.asked != "https://internetdb.shodan.io/203.0.113.7" {
		t.Errorf("an empty setting means the default: asked %q, %v", o.asked, err)
	}
}

func TestOpenPorts_SaysWhenInternetAccessIsOffInsteadOfTheServiceBeingDown(t *testing.T) {
	for name, cause := range map[string]error{"tools off": tools.ErrNetworkDisabled, "internet blocked": tools.ErrInternetBlocked} {
		_, err := clientOf(&opener{err: cause}, DefaultURL).OpenPorts(context.Background(), public)
		if !errors.Is(err, ErrInternetOff) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := clientOf(&opener{err: errors.New("connection refused")}, DefaultURL).OpenPorts(context.Background(), public); errors.Is(err, ErrInternetOff) {
		t.Error("a refused connection is not 'internet off'")
	}
}
