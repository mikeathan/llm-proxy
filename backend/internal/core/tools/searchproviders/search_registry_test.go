package searchproviders

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"llm-proxy/models"
)

// roundTripperFunc and newTestClient are shared by the provider tests
// (search_tavily_test.go, search_brave_test.go, search_serpapi_test.go). They
// live here, with the package's shared provider plumbing they support, rather
// than in a test file with no mirroring source.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// newTestClient returns a client that redirects every request to srv while
// preserving the original path, so a hardcoded provider endpoint stays testable
// without a production test seam.
func newTestClient(srv *httptest.Server) *http.Client {
	target, _ := url.Parse(srv.URL)
	return &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		return http.DefaultTransport.RoundTrip(req)
	})}
}

// TestProviderTable_CoversCanonicalProviders keeps the explicit provider table
// in sync with the canonical enum: every models.SearchProviderIDs() entry must
// have a builder, and an unregistered id must not.
func TestProviderTable_CoversCanonicalProviders(t *testing.T) {
	for _, name := range models.SearchProviderIDs() {
		spec, ok := GetSearchProvider(name)
		if !ok {
			t.Fatalf("%s must be registered in the provider table", name)
		}
		if spec.New == nil {
			t.Errorf("%s spec has a nil constructor", name)
		}
		// Every shipped provider currently requires a key; an intentional change
		// should update this assertion.
		if !spec.RequiresKey {
			t.Errorf("%s must require a key", name)
		}
	}
	if _, ok := GetSearchProvider("bogus"); ok {
		t.Error("unregistered provider returned a spec")
	}
}

func TestNormalizedMaxResults(t *testing.T) {
	tests := []struct {
		in   int
		want int
	}{
		{0, 5},
		{-3, 5},
		{1, 1},
		{5, 5},
		{20, 20},
		{21, 20},
	}
	for _, tt := range tests {
		if got := normalizedMaxResults(tt.in); got != tt.want {
			t.Errorf("normalizedMaxResults(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestValidResultURL(t *testing.T) {
	tests := []struct {
		raw  string
		want bool
	}{
		{"https://example.com/a", true},
		{"http://example.com", true},
		{"", false},
		{"not a url", false},
		{"ftp://example.com", false},
		{"javascript:alert(1)", false},
		{"https://", false},
		{"/relative/path", false},
	}
	for _, tt := range tests {
		if got := validResultURL(tt.raw); got != tt.want {
			t.Errorf("validResultURL(%q) = %v, want %v", tt.raw, got, tt.want)
		}
	}
}
