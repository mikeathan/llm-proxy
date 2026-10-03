package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// stubSearchProvider records calls and returns canned results/errors.
type stubSearchProvider struct {
	results []SearchResult
	err     error
	calls   int
	queries []string
	ranges  []SearchTimeRange
}

func (s *stubSearchProvider) Search(_ context.Context, query string, timeRange SearchTimeRange) ([]SearchResult, error) {
	s.calls++
	s.queries = append(s.queries, query)
	s.ranges = append(s.ranges, timeRange)
	return s.results, s.err
}

func TestInternetToolsSearch_NilSafe(t *testing.T) {
	t.Run("nil receiver returns ErrSearchNotConfigured", func(t *testing.T) {
		var it *InternetTools
		var _ SearchProvider = it // nil receiver still satisfies the interface

		_, err := it.Search(context.Background(), "weather", "")
		if !errors.Is(err, ErrSearchNotConfigured) {
			t.Fatalf("nil receiver: err = %v, want ErrSearchNotConfigured", err)
		}
	})

	t.Run("nil resolver returns ErrSearchNotConfigured", func(t *testing.T) {
		it := NewInternetTools(nil)
		_, err := it.Search(context.Background(), "weather", "")
		if !errors.Is(err, ErrSearchNotConfigured) {
			t.Fatalf("nil resolver: err = %v, want ErrSearchNotConfigured", err)
		}
	})
}

func TestInternetToolsSearch_RejectsEmptyQuery(t *testing.T) {
	it := NewInternetTools(func(context.Context) (SearchProvider, error) {
		t.Fatal("resolver must not run for an empty query")
		return nil, nil
	})
	for _, q := range []string{"", "   ", "\t\n"} {
		if _, err := it.Search(context.Background(), q, ""); err == nil {
			t.Errorf("query %q: expected an error", q)
		}
	}
}

func TestInternetToolsSearch_ResolvesLivePerCall(t *testing.T) {
	resolves := 0
	var providers []*stubSearchProvider
	it := NewInternetTools(func(context.Context) (SearchProvider, error) {
		resolves++
		p := &stubSearchProvider{results: []SearchResult{{Title: "t", URL: "https://example.com", Snippet: "s"}}}
		providers = append(providers, p)
		return p, nil
	})

	for _, q := range []string{"first", "second"} {
		results, err := it.Search(context.Background(), q, "")
		if err != nil {
			t.Fatalf("Search(%q) error = %v", q, err)
		}
		if len(results) != 1 {
			t.Fatalf("Search(%q) returned %d results, want 1", q, len(results))
		}
	}

	if resolves != 2 {
		t.Errorf("resolver ran %d times, want 2 (live resolution per call)", resolves)
	}
	for i, want := range []string{"first", "second"} {
		if providers[i].queries[0] != want {
			t.Errorf("provider %d queried %q, want %q", i, providers[i].queries[0], want)
		}
	}
}

func TestInternetToolsSearch_PropagatesResolverError(t *testing.T) {
	it := NewInternetTools(func(context.Context) (SearchProvider, error) {
		return nil, ErrSearchNotConfigured
	})
	_, err := it.Search(context.Background(), "weather", "")
	if !errors.Is(err, ErrSearchNotConfigured) {
		t.Fatalf("err = %v, want ErrSearchNotConfigured", err)
	}
}

func TestInternetToolsSearch_WrapsProviderError(t *testing.T) {
	sentinel := errors.New("boom")
	p := &stubSearchProvider{err: sentinel}
	it := NewInternetTools(func(context.Context) (SearchProvider, error) { return p, nil })

	_, err := it.Search(context.Background(), "weather", "")
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want to wrap %v", err, sentinel)
	}
	if !strings.Contains(err.Error(), "internet search failed") {
		t.Errorf("err = %v, want operation context", err)
	}
}

func TestInternetToolsSearch_PassesTimeRangeToProvider(t *testing.T) {
	p := &stubSearchProvider{}
	it := NewInternetTools(func(context.Context) (SearchProvider, error) { return p, nil })

	for _, r := range []SearchTimeRange{SearchRangeAny, SearchRangeDay, SearchRangeWeek, SearchRangeMonth, SearchRangeYear} {
		if _, err := it.Search(context.Background(), "llm news", r); err != nil {
			t.Fatalf("range %q: unexpected error %v", r, err)
		}
	}
	want := []SearchTimeRange{"", "day", "week", "month", "year"}
	if len(p.ranges) != len(want) {
		t.Fatalf("ranges = %v, want %v", p.ranges, want)
	}
	for i := range want {
		if p.ranges[i] != want[i] {
			t.Errorf("call %d range = %q, want %q", i, p.ranges[i], want[i])
		}
	}
}

func TestInternetToolsSearch_RejectsUnknownTimeRange(t *testing.T) {
	it := NewInternetTools(func(context.Context) (SearchProvider, error) {
		t.Fatal("resolver must not run for an invalid time range")
		return nil, nil
	})
	_, err := it.Search(context.Background(), "llm news", "fortnight")
	if err == nil || !strings.Contains(err.Error(), "day, week, month, year") {
		t.Fatalf("err = %v, want a message listing the allowed ranges", err)
	}
}

func TestParseSearchTimeRange(t *testing.T) {
	for in, want := range map[string]SearchTimeRange{
		"": SearchRangeAny, "  ": SearchRangeAny, "day": SearchRangeDay, " Week ": SearchRangeWeek, "MONTH": SearchRangeMonth, "year": SearchRangeYear,
	} {
		got, err := ParseSearchTimeRange(in)
		if err != nil || got != want {
			t.Errorf("ParseSearchTimeRange(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := ParseSearchTimeRange("hour"); err == nil {
		t.Error("expected an error for an unknown range")
	}
}
