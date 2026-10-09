package searchproviders

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"llm-proxy/internal/core/tools"
)

const (
	braveSearchURL = "https://api.search.brave.com/res/v1/web/search"
	braveTokenHdr  = "X-Subscription-Token"
	// braveInvalidTokenCode is the error code Brave returns, with status 422 rather than 401, for a rejected key.
	braveInvalidTokenCode = "SUBSCRIPTION_TOKEN_INVALID"
)

// BraveProvider implements tools.SearchProvider using the Brave Search API
// (GET, subscription-token header).
type BraveProvider struct {
	apiKey     string
	client     *http.Client
	maxResults int
}

func newBraveProvider(cfg tools.SearchProviderConfig) (tools.SearchProvider, error) {
	if cfg.Client == nil {
		return nil, fmt.Errorf("brave search client not configured")
	}
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("brave api key missing")
	}
	return &BraveProvider{
		apiKey:     apiKey,
		client:     cfg.Client,
		maxResults: normalizedMaxResults(cfg.MaxResults),
	}, nil
}

// braveFreshness maps our windows to Brave's past-day/week/month/year codes.
var braveFreshness = map[tools.SearchTimeRange]string{
	tools.SearchRangeDay:   "pd",
	tools.SearchRangeWeek:  "pw",
	tools.SearchRangeMonth: "pm",
	tools.SearchRangeYear:  "py",
}

func (b *BraveProvider) Search(ctx context.Context, query string, timeRange tools.SearchTimeRange) ([]tools.SearchResult, error) {
	params := url.Values{}
	params.Set("q", query)
	params.Set("count", strconv.Itoa(b.maxResults))
	if f, ok := braveFreshness[timeRange]; ok {
		params.Set("freshness", f)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, braveSearchURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("brave: create request: %w", err)
	}
	req.Header.Set(braveTokenHdr, b.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brave: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, searchMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("brave: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, braveStatusError(resp.StatusCode, body)
	}

	var raw struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("brave: decode response: %w", err)
	}

	results := make([]tools.SearchResult, 0, len(raw.Web.Results))
	for _, r := range raw.Web.Results {
		if len(results) >= b.maxResults {
			break
		}
		if !validResultURL(r.URL) {
			continue
		}
		results = append(results, tools.SearchResult{Title: r.Title, URL: r.URL, Snippet: r.Description})
	}
	return results, nil
}

// braveStatusError classifies a non-2xx Brave response. Brave reports a rejected key as 422 with
// braveInvalidTokenCode, which the generic 401/403 check would miss; any other 422 is an input problem.
func braveStatusError(code int, body []byte) error {
	var parsed struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if code == http.StatusUnprocessableEntity && json.Unmarshal(body, &parsed) == nil && parsed.Error.Code == braveInvalidTokenCode {
		return credentialRejectedError("brave", code, body)
	}
	return providerStatusError("brave", code, body)
}
