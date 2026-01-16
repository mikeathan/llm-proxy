package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"llm-proxy/internal/nodeherder"
	"llm-proxy/internal/proxy"
	"llm-proxy/utils"
	"strconv"
	"strings"
	"time"
)

type ExposeKey struct {
	device string
	expose string
}

func BuildExposeIndex(ctx *nodeherder.LLMDeviceContext) map[ExposeKey]nodeherder.LLMExpose {
	index := make(map[ExposeKey]nodeherder.LLMExpose, 64)

	for _, d := range ctx.Devices {
		name := strings.ToLower(d.Name)
		for _, e := range d.Exposes {
			index[ExposeKey{
				device: name,
				expose: strings.ToLower(e.Name),
			}] = e
		}
	}

	return index
}

// Intent captures the user-facing intent so the backend can deterministically
// map it into metrics queries without the LLM choosing execution parameters.
type Intent struct {
	Intent          string   `json:"intent"`
	TargetName      string   `json:"target_name"`
	Metrics         []string `json:"metrics"`
	TimeScope       string   `json:"time_scope"`
	PositiveOutcome *bool    `json:"positive_outcome,omitempty"`
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
						"description": "Intent such as count_events, latest_value (current status only), last_event (find when something happened), min_value, max_value, avg_value",
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
						"description": "Time scope such as today, yesterday, last_24_hours, last_week, or range:<from>..<to>",
					},
					"positive_outcome": map[string]any{
						"type":        "boolean",
						"description": "For binary sensors, use 'true' if the user asks when a specific event happened (e.g. 'when did it open', 'when was it turned on', 'when was presence detected') to find that specific state change. Use 'false' for negative events ('when did it close', 'when did it turn off'). Omit if asking for the current status.",
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

func ValidateIntent(intent Intent, index map[ExposeKey]nodeherder.LLMExpose) error {

	intent.Intent = strings.ToLower(strings.TrimSpace(intent.Intent))
	intent.TimeScope = strings.ToLower(strings.TrimSpace(intent.TimeScope))

	for _, m := range intent.Metrics {
		key := ExposeKey{
			device: strings.ToLower(intent.TargetName),
			expose: strings.ToLower(m),
		}

		expose, ok := index[key]
		if !ok {
			return fmt.Errorf("metric '%s' not found on device '%s'", m, intent.TargetName)
		}

		isNumeric := expose.Type == "numeric" || expose.Type == "number" ||
			expose.Type == "float" || expose.Type == "integer"

		if isNumeric && intent.Intent == "count_events" {
			return fmt.Errorf("count_events is invalid for numeric sensor: %s", m)
		}

		if !isNumeric && intent.Intent == "latest_value" &&
			strings.Contains(intent.TimeScope, "change") {
			return fmt.Errorf("latest_value cannot answer change questions for event sensor: %s", m)
		}
	}
	// Rule 1: latest_value cannot answer any historical event question
	// if intent.Intent == "latest_value" {
	// 	for _, m := range intent.Metrics {
	// 		key := ExposeKey{
	// 			device: strings.ToLower(intent.TargetName),
	// 			expose: strings.ToLower(m),
	// 		}
	// 		expose := index[key]

	// 		isNumeric := expose.Type == "numeric" || expose.Type == "number" ||
	// 			expose.Type == "float" || expose.Type == "integer"

	// 		if !isNumeric {
	// 			switch intent.TimeScope {
	// 			case "today", "yesterday", "last_hour", "last_day", "last_week", "last_month", "last_year":
	// 				return errors.New("Use intent = count_events for event-based history queries")
	// 			}
	// 		}
	// 	}
	// }

	// Rule 2: latest_value is invalid for explicit ranges (change/comparison)
	if intent.Intent == "latest_value" && strings.HasPrefix(intent.TimeScope, "range:") {
		return errors.New("Use intent = count_events or remove range when requesting latest value")
	}

	// Rule 3: count_events requires time_scope
	if intent.Intent == "count_events" && intent.TimeScope == "" {
		return errors.New("count_events requires a time_scope such as today, yesterday, last_day, or range")
	}

	return nil
}

// IntentToMetricsArgs deterministically maps a declared intent into concrete
// query_metrics arguments so execution is controlled by code, not the LLM.
func IntentToMetricsArgs(intent Intent, clock utils.Clock, exposeIndex map[ExposeKey]nodeherder.LLMExpose, timezone string) ([]MetricsArgs, error) {
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
	timeArgs := timeArgsForScope(intent.TimeScope, clock, timezone)

	out := make([]MetricsArgs, 0, len(metrics))
	for _, metric := range metrics {
		args := MetricsArgs{
			TargetName:  target,
			Expose:      metric,
			Time:        timeArgs,
			Aggregation: aggregation,
		}

		// Implicitly default to finding the positive event (true/open) if user asks for last_event
		// but doesn't specify outcome. This handles "wen did it open" queries where LLM forgets to set positive_outcome.
		if aggregation == "last_event" && intent.PositiveOutcome == nil {
			val := true
			args.PositiveOutcome = &val
		}

		key := ExposeKey{
			device: strings.ToLower(intent.TargetName),
			expose: strings.ToLower(metric),
		}
		expose, ok := exposeIndex[key]
		isNumeric := expose.Type == "numeric" || expose.Type == "number" ||
			expose.Type == "float" || expose.Type == "integer"

		if ok && !isNumeric && intent.PositiveOutcome != nil {
			args.Aggregation = "none"
			args.PositiveOutcome = intent.PositiveOutcome
		}

		out = append(out, args)
	}

	return out, nil
}

func aggregationForIntent(raw string) string {
	switch raw {
	case "count_events", "count":
		return "count"
	case "latest_value", "last_value", "last":
		return "last"
	case "last_event":
		return "last_event"
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

func timeArgsForScope(raw string, clock utils.Clock, timezone string) *TimeArgs {
	scope := strings.ToLower(strings.TrimSpace(raw))
	if scope == "" {
		return nil
	}

	// Exact match first
	switch scope {
	case "today":
		return timeArgsForToday(clock, timezone)
	case "yesterday":
		return timeArgsForYesterday(clock, timezone)
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

	// Fallback/Fuzzy matching logic for when LLM combines terms like "last_day yesterday"
	if strings.Contains(scope, "yesterday") {
		return timeArgsForYesterday(clock, timezone)
	}
	if strings.Contains(scope, "today") {
		return timeArgsForToday(clock, timezone)
	}

	if _, ok := NormalizeLookback(scope, DefaultNormalizeConfig()); ok {
		return &TimeArgs{Lookback: scope}
	}

	return nil
}

func timeArgsForToday(clock utils.Clock, timezone string) *TimeArgs {
	now := clock.NowUtc()
	loc, err := time.LoadLocation(timezone)
	if err != nil || timezone == "" {
		loc = time.UTC
	}

	nowInLoc := now.In(loc)
	start := time.Date(nowInLoc.Year(), nowInLoc.Month(), nowInLoc.Day(), 0, 0, 0, 0, loc)
	return &TimeArgs{
		From: start.UTC().UnixMilli(),
		To:   now.UnixMilli(),
	}
}

func timeArgsForYesterday(clock utils.Clock, timezone string) *TimeArgs {
	now := clock.NowUtc()
	loc, err := time.LoadLocation(timezone)
	if err != nil || timezone == "" {
		loc = time.UTC
	}

	nowInLoc := now.In(loc)
	startToday := time.Date(nowInLoc.Year(), nowInLoc.Month(), nowInLoc.Day(), 0, 0, 0, 0, loc)
	startYesterday := startToday.Add(-24 * time.Hour)
	return &TimeArgs{
		From: startYesterday.UTC().UnixMilli(),
		To:   startToday.UTC().UnixMilli(),
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
