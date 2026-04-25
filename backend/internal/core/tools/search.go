package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

// TavilyProvider implements the SearchProvider using the Tavily API.
type TavilyProvider struct {
	APIKey string
	Client *http.Client
}

func (t *TavilyProvider) Search(ctx context.Context, query string) ([]SearchResult, error) {
	if t.APIKey == "" {
		return nil, fmt.Errorf("tavily api key missing")
	}

	client := t.Client
	if client == nil {
		client = http.DefaultClient
	}

	const searchURL = "https://api.tavily.com/search"
	
	payload := map[string]any{
		"api_key":      t.APIKey,
		"query":        query,
		"search_depth": "basic",
		"max_results":  5,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal search payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", searchURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create search request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errData map[string]any
		json.NewDecoder(resp.Body).Decode(&errData)
		return nil, fmt.Errorf("tavily API error (status %d): %v", resp.StatusCode, errData)
	}

	var rawResponse struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rawResponse); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}

	// Map to our generic SearchResult format
	results := make([]SearchResult, 0, len(rawResponse.Results))
	for _, r := range rawResponse.Results {
		results = append(results, SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Content,
		})
	}

	return results, nil
}

// InternetTools provides access to search capabilities.
type InternetTools struct {
	provider SearchProvider
}

func NewInternetTools(p SearchProvider) *InternetTools {
	return &InternetTools{provider: p}
}

func (i *InternetTools) Search(ctx context.Context, query string) ([]SearchResult, error) {
	if i.provider == nil {
		return nil, fmt.Errorf("no search provider configured")
	}
	return i.provider.Search(ctx, query)
}
