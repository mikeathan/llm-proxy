package stackport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/core/tools"
)

type opener struct {
	body   string
	err    error
	gotURL string
	gotAcc string
}

func (o *opener) OpenAccepting(_ context.Context, target, accept string) (*http.Response, error) {
	o.gotURL, o.gotAcc = target, accept
	if o.err != nil {
		return nil, o.err
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(o.body))}, nil
}

func TestFetch_ReturnsTheBodyAndPassesAcceptAndAddressOn(t *testing.T) {
	o := &opener{body: "hello"}
	got, err := Fetch{Opener: o}.Get(context.Background(), stacksdk.FetchRequest{URL: "https://example.org/x", Accept: "application/dns-json"})
	if err != nil || string(got.Body) != "hello" || got.Truncated {
		t.Fatalf("got %+v, %v", got, err)
	}
	if o.gotURL != "https://example.org/x" || o.gotAcc != "application/dns-json" {
		t.Errorf("opener saw %q, %q", o.gotURL, o.gotAcc)
	}
}

func TestFetch_CutsTheBodyAtTheRequestsLimitAndSaysSo(t *testing.T) {
	got, err := Fetch{Opener: &opener{body: "0123456789"}}.Get(context.Background(), stacksdk.FetchRequest{URL: "https://example.org", MaxBytes: 4})
	if err != nil || string(got.Body) != "0123" || !got.Truncated {
		t.Fatalf("got %+v, %v", got, err)
	}
	exact, _ := Fetch{Opener: &opener{body: "0123"}}.Get(context.Background(), stacksdk.FetchRequest{URL: "https://example.org", MaxBytes: 4})
	if string(exact.Body) != "0123" || exact.Truncated {
		t.Errorf("a body of exactly the limit is not cut: %+v", exact)
	}
}

func TestFetch_NeverReadsMoreThanTheCeiling(t *testing.T) {
	big := strings.Repeat("a", maxBodyCeil+10)
	got, _ := Fetch{Opener: &opener{body: big}}.Get(context.Background(), stacksdk.FetchRequest{URL: "https://example.org", MaxBytes: maxBodyCeil * 4})
	if len(got.Body) != maxBodyCeil || !got.Truncated {
		t.Errorf("read %d bytes, truncated=%v", len(got.Body), got.Truncated)
	}
}

func TestFetch_FailuresCrossAsCodesWithFixedWords(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want stacksdk.Code
	}{
		{"network off", tools.ErrNetworkDisabled, stacksdk.CodeUnavailable},
		{"internet off", fmt.Errorf("blocked: %w", tools.ErrInternetBlocked), stacksdk.CodeUnavailable},
		{"lan blocked", tools.ValidateDomainBoundary("a.bad.example", []string{"bad.example"}), stacksdk.CodeDenied},
		{"timeout", context.DeadlineExceeded, stacksdk.CodeTimeout},
		{"not 200", &tools.StatusError{Code: 503, Status: "503 Service Unavailable"}, stacksdk.CodeInternal},
		{"anything else", errors.New("dial tcp 10.1.2.3:443: refused"), stacksdk.CodeInternal},
	}
	for _, c := range cases {
		_, err := Fetch{Opener: &opener{err: c.err}}.Get(context.Background(), stacksdk.FetchRequest{URL: "https://example.org"})
		if stacksdk.CodeOf(err) != c.want {
			t.Errorf("%s: code = %q, want %q (%v)", c.name, stacksdk.CodeOf(err), c.want, err)
		}
		if err != nil && (strings.Contains(err.Error(), "10.1.2.3") || strings.Contains(err.Error(), "bad.example")) {
			t.Errorf("%s: the message repeats the guard's or transport's words: %v", c.name, err)
		}
	}
}

func TestFetch_RefusesAnAddressThatIsNotHTTPWithoutAsking(t *testing.T) {
	o := &opener{}
	for _, u := range []string{"", "file:///etc/passwd", "ftp://example.org", "https://", "::bad"} {
		_, err := Fetch{Opener: o}.Get(context.Background(), stacksdk.FetchRequest{URL: u})
		if stacksdk.CodeOf(err) != stacksdk.CodeInvalid {
			t.Errorf("%q: code = %q", u, stacksdk.CodeOf(err))
		}
	}
	if o.gotURL != "" {
		t.Errorf("the guard was asked about %q", o.gotURL)
	}
}
