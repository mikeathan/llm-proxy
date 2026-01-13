package tools

import (
	"encoding/json"
	"fmt"
	"llm-proxy/internal/proxy"
	"llm-proxy/utils"
	"strconv"
	"strings"
	"time"
)

// Intent captures the user-facing intent so the backend can deterministically
// map it into metrics queries without the LLM choosing execution parameters.
type Intent struct {
	Intent     string   `json:"intent"`
	TargetName string   `json:"target_name"`
	Metrics    []string `json:"metrics"`
	TimeScope  string   `json:"time_scope"`
}

func IntentToolSchema() proxy.Tool {
	return proxy.Tool{
		Type: "function",
		Function: proxy.FunctionSchema{
			Name:        "declare_intent",
			Description: "Declare the user's metrics intent; backend resolves aggregation/time deterministically",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"intent": map[string]any{
						"type":        "string",
						"description": "Intent such as count_events, latest_value, min_value, max_value, avg_value",
					},
					"target_name": map[string]any{
						"type":        "string",
						"description": "Natural language device name, e.g. 'garage', 'living room'",
					},
					"metrics": map[string]any{
						"type":        "array",
						"description": "One or more metric keys from device.exposes.name",
						"items": map[string]any{
							"type": "string",
						},
					},
					"time_scope": map[string]any{
						"type":        "string",
						"description": "Time scope such as today, last_hour, last_day, last_week, or range:<from>..<to>",
					},
				},
				"required": []string{"intent", "target_name", "metrics", "time_scope"},
			},
		},
	}
}

func ParseIntentArgs(raw string) (Intent, error) {
	var args Intent
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return Intent{}, err
	}
	return args, nil
}

func ValidateIntent(intent Intent) error {

	// Rule 1: latest_value cannot answer any historical event question
	if intent.Intent == "latest_value" {
		switch intent.TimeScope {
		case "today", "yesterday", "last_hour", "last_day", "last_week", "last_month", "last_year":
			return fmt.Errorf("latest_value cannot satisfy historical or event-based queries")
		}
	}

	// Rule 2: change-related queries require more than one sample
	if intent.Intent == "latest_value" && strings.HasPrefix(intent.TimeScope, "range:") {
		return fmt.Errorf("latest_value invalid for range queries")
	}

	// Rule 3: count_events requires a time scope
	if intent.Intent == "count_events" && intent.TimeScope == "" {
		return fmt.Errorf("count_events requires explicit time_scope")
	}

	return nil
}

// IntentToMetricsArgs deterministically maps a declared intent into concrete
// query_metrics arguments so execution is controlled by code, not the LLM.
func IntentToMetricsArgs(intent Intent, clock utils.Clock) ([]MetricsArgs, error) {
	target := strings.TrimSpace(intent.TargetName)
	if target == "" {
		return nil, fmt.Errorf("target_name is required")
	}

	metrics := make([]string, 0, len(intent.Metrics))
	for _, metric := range intent.Metrics {
		metric = strings.TrimSpace(metric)
		if metric != "" {
			metrics = append(metrics, metric)
		}
	}
	if len(metrics) == 0 {
		return nil, fmt.Errorf("metrics is required")
	}

	aggregation := aggregationForIntent(intent.Intent)
	timeArgs := timeArgsForScope(intent.TimeScope, clock)

	out := make([]MetricsArgs, 0, len(metrics))
	for _, metric := range metrics {
		out = append(out, MetricsArgs{
			TargetName:  target,
			Expose:      metric,
			Time:        timeArgs,
			Aggregation: aggregation,
		})
	}

	return out, nil
}

func aggregationForIntent(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "count_events", "count":
		return "count"
	case "latest_value", "last_value", "last":
		return "last"
	case "min_value", "minimum", "min":
		return "min"
	case "max_value", "maximum", "max":
		return "max"
	case "avg_value", "average", "avg", "mean":
		return "avg"
	default:
		return "last"
	}
}

func timeArgsForScope(raw string, clock utils.Clock) *TimeArgs {
	scope := strings.ToLower(strings.TrimSpace(raw))
	if scope == "" {
		return nil
	}

	switch scope {
	case "today":
		return timeArgsForToday(clock)
	case "yesterday":
		return timeArgsForYesterday(clock)
	case "last_hour":
		return &TimeArgs{Lookback: "1h"}
	case "last_day", "last_24_hours":
		return &TimeArgs{Lookback: "24h"}
	case "last_week", "last_7_days":
		return &TimeArgs{Lookback: "168h"}
	case "last_month", "last_30_days":
		return &TimeArgs{Lookback: "720h"}
	case "last_year", "last_365_days":
		return &TimeArgs{Lookback: "8760h"}
	}

	if strings.HasPrefix(scope, "range:") {
		return parseRangeScope(strings.TrimPrefix(scope, "range:"), clock)
	}

	if _, ok := NormalizeLookback(scope, DefaultNormalizeConfig()); ok {
		return &TimeArgs{Lookback: scope}
	}

	return nil
}

func timeArgsForToday(clock utils.Clock) *TimeArgs {
	now := clock.NowUtc()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return &TimeArgs{
		From: start.UnixMilli(),
		To:   now.UnixMilli(),
	}
}

func timeArgsForYesterday(clock utils.Clock) *TimeArgs {
	now := clock.NowUtc()
	startToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	startYesterday := startToday.Add(-24 * time.Hour)
	return &TimeArgs{
		From: startYesterday.UnixMilli(),
		To:   startToday.UnixMilli(),
	}
}

func parseRangeScope(raw string, clock utils.Clock) *TimeArgs {
	parts := strings.SplitN(raw, "..", 2)
	if len(parts) != 2 {
		return nil
	}

	from, ok := parseTimeBound(parts[0], clock)
	if !ok {
		return nil
	}
	to, ok := parseTimeBound(parts[1], clock)
	if !ok {
		return nil
	}

	return &TimeArgs{
		From: from.UnixMilli(),
		To:   to.UnixMilli(),
	}
}

func parseTimeBound(raw string, clock utils.Clock) (time.Time, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, false
	}
	if strings.EqualFold(trimmed, "now") {
		return clock.NowUtc(), true
	}

	if ts, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		if ts > 1e12 {
			return time.UnixMilli(ts).UTC(), true
		}
		return time.Unix(ts, 0).UTC(), true
	}

	if t, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
		return t.UTC(), true
	}
	if t, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return t.UTC(), true
	}

	return time.Time{}, false
}
