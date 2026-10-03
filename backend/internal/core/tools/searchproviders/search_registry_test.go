package searchproviders

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"llm-proxy/internal/core/tools"
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

// Each provider expresses the recency window in its own vocabulary; an unset
// range must send no filter at all (a stray default would silently narrow every
// search).
func TestProviders_MapTimeRange(t *testing.T) {
	ranges := []struct {
		in                  tools.SearchTimeRange
		tavily, brave, serp string
	}{
		{tools.SearchRangeAny, "", "", ""},
		{tools.SearchRangeDay, "day", "pd", "qdr:d"},
		{tools.SearchRangeWeek, "week", "pw", "qdr:w"},
		{tools.SearchRangeMonth, "month", "pm", "qdr:m"},
		{tools.SearchRangeYear, "year", "py", "qdr:y"},
	}

	for _, rc := range ranges {
		t.Run("range="+string(rc.in), func(t *testing.T) {
			var tavilyBody map[string]any
			var braveQ, serpQ map[string][]string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/search": // tavily
					raw, _ := io.ReadAll(r.Body)
					tavilyBody = map[string]any{}
					_ = json.Unmarshal(raw, &tavilyBody)
					io.WriteString(w, `{"results":[]}`)
				case "/res/v1/web/search":
					braveQ = r.URL.Query()
					io.WriteString(w, `{"web":{"results":[]}}`)
				default:
					serpQ = r.URL.Query()
					io.WriteString(w, `{"organic_results":[]}`)
				}
			}))
			defer srv.Close()
			cfg := tools.SearchProviderConfig{APIKey: "k", Client: newTestClient(srv)}

			tav, _ := newTavilyProvider(cfg)
			brv, _ := newBraveProvider(cfg)
			srp, _ := newSerpAPIProvider(cfg)
			for name, p := range map[string]tools.SearchProvider{"tavily": tav, "brave": brv, "serpapi": srp} {
				if _, err := p.Search(context.Background(), "llm news", rc.in); err != nil {
					t.Fatalf("%s: %v", name, err)
				}
			}

			if got, _ := tavilyBody["time_range"].(string); got != rc.tavily {
				t.Errorf("tavily time_range = %q, want %q", got, rc.tavily)
			}
			if got := braveQ["freshness"]; (rc.brave == "") != (len(got) == 0) || (len(got) > 0 && got[0] != rc.brave) {
				t.Errorf("brave freshness = %v, want %q", got, rc.brave)
			}
			if got := serpQ["tbs"]; (rc.serp == "") != (len(got) == 0) || (len(got) > 0 && got[0] != rc.serp) {
				t.Errorf("serpapi tbs = %v, want %q", got, rc.serp)
			}
		})
	}
}
