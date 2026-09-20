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

func (b *BraveProvider) Search(ctx context.Context, query string) ([]tools.SearchResult, error) {
	params := url.Values{}
	params.Set("q", query)
	params.Set("count", strconv.Itoa(b.maxResults))

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
		return nil, providerStatusError("brave", resp.StatusCode, body)
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
