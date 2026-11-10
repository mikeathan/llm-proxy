package footprint

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"llm-proxy/internal/core/tools"
)

// fakeOpener answers a GET from a table keyed by URL; an unlisted URL is a service that is down. It records every request.
type fakeOpener struct {
	answers  map[string]string
	failures map[string]error
	requests []string
	headers  []map[string]string
}

func (f *fakeOpener) OpenWith(_ context.Context, target, _ string, headers map[string]string) (*http.Response, error) {
	f.requests = append(f.requests, target)
	f.headers = append(f.headers, headers)
	if err := f.failures[target]; err != nil {
		return nil, err
	}
	body, ok := f.answers[target]
	if !ok {
		return nil, &tools.StatusError{Code: 502, Status: "502 Bad Gateway"}
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func wantReason(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || ReasonOf(err) != want {
		t.Errorf("want reason %q, got %v", want, err)
	}
}
