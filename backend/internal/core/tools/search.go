package tools

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"llm-proxy/models"
)

// SearchProvider defines the interface for various search engines.
type SearchProvider interface {
	Search(ctx context.Context, query string) ([]SearchResult, error)
}

type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// SearchProviderConfig groups the inputs every provider constructor needs. The
// client is the injected guardrailed NetworkTools.HTTPClient() (Constitution
// I.2) — providers never construct their own transport.
type SearchProviderConfig struct {
	APIKey     string
	Client     *http.Client
	MaxResults int
}

// SearchProviderSpec describes a registered provider: whether it requires an API
// key, and the factory that builds it. Constructors never take a context. The
// concrete providers and the explicit registration table live in the
// tools/searchproviders subpackage (mirrors the tools/notifiers split).
type SearchProviderSpec struct {
	RequiresKey bool
	New         func(cfg SearchProviderConfig) (SearchProvider, error)
}

// ErrSearchNotConfigured means no search provider is usable. It wraps
// models.ErrToolUnavailable so the loop treats it as terminal (do not retry).
var ErrSearchNotConfigured = fmt.Errorf("internet search is not configured: %w", models.ErrToolUnavailable)

// ProviderResolver resolves the live provider for a call. It reads the current
// config + secret on every invocation, so an operator change applies without a
// restart.
type ProviderResolver func(ctx context.Context) (SearchProvider, error)

// InternetTools exposes the internet_search tool. It is nil-safe: a nil receiver
// or nil resolver is the Null Object that returns ErrSearchNotConfigured rather
// than panicking, so callers never need a nil check (a residual call after the
// schema-hide gate reaches the tool and gets a clear error).
type InternetTools struct {
	resolve ProviderResolver
}

func NewInternetTools(resolve ProviderResolver) *InternetTools {
	return &InternetTools{resolve: resolve}
}

// Search validates the query, resolves the live provider, and runs the search.
// Provider errors are wrapped so the loop records them as a non-approvable tool
// result.
func (i *InternetTools) Search(ctx context.Context, query string) ([]SearchResult, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("search query must not be empty")
	}
	if i == nil || i.resolve == nil {
		return nil, ErrSearchNotConfigured
	}
	provider, err := i.resolve(ctx)
	if err != nil {
		return nil, err
	}
	results, err := provider.Search(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("internet search failed: %w", err)
	}
	return results, nil
}
