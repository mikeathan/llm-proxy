package searchproviders

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"llm-proxy/internal/core/tools"
)

const (
	tavilySearchURL   = "https://api.tavily.com/search"
	tavilySearchDepth = "basic"
	tavilyAuthScheme  = "Bearer "
)

// TavilyProvider implements tools.SearchProvider using the Tavily API (POST,
// JSON body, `Authorization: Bearer` key).
type TavilyProvider struct {
	apiKey     string
	client     *http.Client
	maxResults int
}

// newTavilyProvider builds the provider from the injected config. A nil client or
// missing key is a programming/configuration error, never a silent fallback to
// http.DefaultClient (Constitution I.1).
func newTavilyProvider(cfg tools.SearchProviderConfig) (tools.SearchProvider, error) {
	if cfg.Client == nil {
		return nil, fmt.Errorf("tavily search client not configured")
	}
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("tavily api key missing")
	}
	return &TavilyProvider{
		apiKey:     apiKey,
		client:     cfg.Client,
		maxResults: normalizedMaxResults(cfg.MaxResults),
	}, nil
}

func (t *TavilyProvider) Search(ctx context.Context, query string) ([]tools.SearchResult, error) {
	payload := map[string]any{
		"query":        query,
		"search_depth": tavilySearchDepth,
		"max_results":  t.maxResults,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("tavily: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tavilySearchURL, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("tavily: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// Tavily authenticates via the Authorization header; the legacy body
	// `api_key` field is no longer accepted (401 "missing or invalid API key").
	req.Header.Set("Authorization", tavilyAuthScheme+t.apiKey)

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tavily: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, searchMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("tavily: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, providerStatusError("tavily", resp.StatusCode, body)
	}

	var raw struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("tavily: decode response: %w", err)
	}

	results := make([]tools.SearchResult, 0, len(raw.Results))
	for _, r := range raw.Results {
		if len(results) >= t.maxResults {
			break
		}
		if !validResultURL(r.URL) {
			continue
		}
		results = append(results, tools.SearchResult{Title: r.Title, URL: r.URL, Snippet: r.Content})
	}
	return results, nil
}
