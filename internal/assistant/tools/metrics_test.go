package tools_test

import (
	"testing"
	"time"

	"llm-proxy/internal/assistant/tools"
)

type fixedClock struct {
	now time.Time
}

func (f fixedClock) Now() time.Time {
	return f.now
}

func TestNormalizeExpose(t *testing.T) {
	if got := tools.NormalizeExpose("temperature (°C)"); got != "temperature" {
		t.Fatalf("expected temperature, got %q", got)
	}
	if got := tools.NormalizeExpose("battery level"); got != "battery_level" {
		t.Fatalf("expected battery_level, got %q", got)
	}
}

func TestNormalizeLookback(t *testing.T) {
	cfg := tools.DefaultNormalizeConfig()

	cases := []struct {
		input  string
		wantOK bool
		want   time.Duration
	}{
		{input: "day", wantOK: true, want: 24 * time.Hour},
		{input: "1day", wantOK: true, want: 24 * time.Hour},
		{input: "10 years", wantOK: true, want: cfg.MaxExplicitLookback},
		{input: "max", wantOK: false},
		{input: "nonsense", wantOK: false},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, ok := tools.NormalizeLookback(tc.input, cfg)
			if ok != tc.wantOK {
				t.Fatalf("expected ok=%v, got %v", tc.wantOK, ok)
			}
			if tc.wantOK && got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestNormalizeTimeQuery_DefaultLookback(t *testing.T) {
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	cfg := tools.DefaultNormalizeConfig()
	query := tools.NormalizeTimeQuery(&tools.TimeArgs{}, cfg, fixedClock{now: now})
	if query == nil {
		t.Fatal("expected time query")
	}
	if query.From.IsZero() || query.To.IsZero() {
		t.Fatalf("expected from/to to be set")
	}
	if query.To != now {
		t.Fatalf("expected to=%v, got %v", now, query.To)
	}
	if query.From != now.Add(-cfg.DefaultLookback) {
		t.Fatalf("expected from=%v, got %v", now.Add(-cfg.DefaultLookback), query.From)
	}
}

func TestNormalizeMetricsArgs_NormalizesExposeAndLookback(t *testing.T) {
	args := tools.MetricsArgs{
		TargetName: "Kitchen",
		Expose:     "temperature (°C)",
		Time:       &tools.TimeArgs{Lookback: "max"},
	}
	cfg := tools.DefaultNormalizeConfig()
	out, err := tools.NormalizeMetricsArgs(args, cfg, fixedClock{now: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Expose != "temperature" {
		t.Fatalf("expected temperature, got %q", out.Expose)
	}
	if out.Time == nil || out.Time.From.IsZero() || out.Time.To.IsZero() {
		t.Fatalf("expected default time range")
	}
}

func TestNormalizeTimeQuery_Boundaries(t *testing.T) {
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := tools.DefaultNormalizeConfig()
	clock := fixedClock{now: now}

	cases := []struct {
		name      string
		args      *tools.TimeArgs
		wantEmpty bool
	}{
		{name: "negative", args: &tools.TimeArgs{From: -1, To: -2}, wantEmpty: false},
		{name: "future", args: &tools.TimeArgs{From: now.Add(2 * time.Hour).UnixMilli()}, wantEmpty: false},
		{name: "swap", args: &tools.TimeArgs{From: now.Add(-2 * time.Hour).UnixMilli(), To: now.Add(-3 * time.Hour).UnixMilli()}, wantEmpty: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := tools.NormalizeTimeQuery(tc.args, cfg, clock)
			if q == nil {
				t.Fatalf("expected time query")
			}
			if q.From.IsZero() || q.To.IsZero() {
				t.Fatalf("expected normalized bounds")
			}
		})
	}
}
