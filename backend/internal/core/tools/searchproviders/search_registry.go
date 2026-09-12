// Package searchproviders holds the concrete internet-search provider
// implementations and the explicit registration table. The contract
// (SearchProvider, SearchProviderConfig, SearchResult, InternetTools) stays in
// the parent tools package — mirroring the tools/notifiers split: this
// subpackage imports the parent for its contract, and the parent never imports
// this package (the table lives here, so root tools needs nothing from us).
package searchproviders

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"llm-proxy/internal/core/tools"
	"llm-proxy/models"
)

// searchMaxResponseBytes bounds provider response decoding (success and error
// paths) so a hostile or broken endpoint cannot stream unbounded data into memory.
const searchMaxResponseBytes = 1 << 20

// errBodyMaxLen caps a provider error body included in a wrapped error.
const errBodyMaxLen = 512

// searchProviderSpecs is the explicit provider table — the single place a
// provider is registered. Deliberately no init() self-registration: adding a
// provider is one constructor file plus one row here (engineering-practices
// "No init()"). GetSearchProvider is the read seam the wiring uses.
var searchProviderSpecs = map[models.SearchProvider]tools.SearchProviderSpec{
	models.SearchProviderTavily:  {RequiresKey: true, New: newTavilyProvider},
	models.SearchProviderBrave:   {RequiresKey: true, New: newBraveProvider},
	models.SearchProviderSerpAPI: {RequiresKey: true, New: newSerpAPIProvider},
}

// GetSearchProvider returns the registered spec for name, if any. The provider
// set is fixed (models.SearchProviderIDs); adding one is a table row above, not
// runtime registration.
func GetSearchProvider(name models.SearchProvider) (tools.SearchProviderSpec, bool) {
	spec, ok := searchProviderSpecs[name]
	return spec, ok
}

// isAuthStatus reports whether an HTTP status is an authentication/authorization
// failure — operator-actionable (the provider rejected the credential).
func isAuthStatus(code int) bool {
	return code == http.StatusUnauthorized || code == http.StatusForbidden
}

// providerStatusError builds the error for a non-2xx provider response. Auth
// failures additionally wrap models.ErrToolUnavailable so the agent loop
// classifies them as terminal (do not retry), while other statuses stay plain
// (transient/input — model-actionable).
func providerStatusError(provider string, code int, body []byte) error {
	err := fmt.Errorf("%s API error (status %d): %s", provider, code, errorBodySnippet(body))
	if isAuthStatus(code) {
		return fmt.Errorf("%w: %v", models.ErrToolUnavailable, err)
	}
	return err
}

// normalizedMaxResults applies the search result bounds at read time: unset
// (zero) falls back to the default and an out-of-range value is clamped, so a
// hand-edited registry.json can never make a provider request unbounded results.
func normalizedMaxResults(n int) int {
	switch {
	case n <= 0:
		return models.DefaultSearchMaxResults
	case n > models.MaxSearchMaxResults:
		return models.MaxSearchMaxResults
	default:
		return n
	}
}

// validResultURL reports whether raw is an absolute http(s) URL. Provider
// payloads are untrusted; malformed or non-http(s) entries are skipped rather
// than surfaced to the agent.
func validResultURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// errorBodySnippet trims and caps a provider error body for a wrapped error, so
// an unbounded or binary error page cannot bloat the message.
func errorBodySnippet(body []byte) string {
	s := strings.TrimSpace(string(body))
	if len(s) > errBodyMaxLen {
		return s[:errBodyMaxLen] + "..."
	}
	return s
}
