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
	serpAPIURL    = "https://serpapi.com/search.json"
	serpAPIEngine = "google"
)

// SerpAPIProvider implements tools.SearchProvider using the SerpAPI Google
// search engine (GET, api_key query param).
type SerpAPIProvider struct {
	apiKey     string
	client     *http.Client
	maxResults int
}

func newSerpAPIProvider(cfg tools.SearchProviderConfig) (tools.SearchProvider, error) {
	if cfg.Client == nil {
		return nil, fmt.Errorf("serpapi search client not configured")
	}
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("serpapi api key missing")
	}
	return &SerpAPIProvider{
		apiKey:     apiKey,
		client:     cfg.Client,
		maxResults: normalizedMaxResults(cfg.MaxResults),
	}, nil
}

func (s *SerpAPIProvider) Search(ctx context.Context, query string) ([]tools.SearchResult, error) {
	params := url.Values{}
	params.Set("engine", serpAPIEngine)
	params.Set("q", query)
	params.Set("api_key", s.apiKey)
	params.Set("num", strconv.Itoa(s.maxResults))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serpAPIURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("serpapi: create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("serpapi: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, searchMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("serpapi: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, providerStatusError("serpapi", resp.StatusCode, body)
	}

	// SerpAPI can report failures as a 200 with an `error` field.
	var raw struct {
		Error   string `json:"error"`
		Results []struct {
			Title   string `json:"title"`
			Link    string `json:"link"`
			Snippet string `json:"snippet"`
		} `json:"organic_results"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("serpapi: decode response: %w", err)
	}
	if raw.Error != "" {
		return nil, fmt.Errorf("serpapi API error: %s", errorBodySnippet([]byte(raw.Error)))
	}

	results := make([]tools.SearchResult, 0, len(raw.Results))
	for _, r := range raw.Results {
		if len(results) >= s.maxResults {
			break
		}
		if !validResultURL(r.Link) {
			continue
		}
		results = append(results, tools.SearchResult{Title: r.Title, URL: r.Link, Snippet: r.Snippet})
	}
	return results, nil
}
