package tools

import (
	"encoding/json"
	"fmt"
	"llm-proxy/internal/nodeherder"
	"llm-proxy/internal/proxy"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultSummaryMaxLen       = 4000
	defaultLookback            = 24 * time.Hour
	defaultMaxLookback         = 30 * 24 * time.Hour
	defaultMaxExplicitLookback = 365 * 24 * time.Hour
	defaultMaxFutureSkew       = time.Hour
)

type Clock interface {
	Now() time.Time
}

type RealClock struct{}

func (RealClock) Now() time.Time {
	return time.Now().UTC()
}

type NormalizeConfig struct {
	DefaultLookback     time.Duration
	MaxLookback         time.Duration
	MaxExplicitLookback time.Duration
	MaxFutureSkew       time.Duration
}

func DefaultNormalizeConfig() NormalizeConfig {
	return NormalizeConfig{
		DefaultLookback:     defaultLookback,
		MaxLookback:         defaultMaxLookback,
		MaxExplicitLookback: defaultMaxExplicitLookback,
		MaxFutureSkew:       defaultMaxFutureSkew,
	}
}

type MetricsArgs struct {
	TargetName  string    `json:"target_name"`
	Expose      string    `json:"expose"`
	Time        *TimeArgs `json:"time,omitempty"`
	Aggregation string    `json:"aggregation,omitempty"`
	Resolution  string    `json:"resolution,omitempty"`
}

type TimeArgs struct {
	From     int64  `json:"from,omitempty"`
	To       int64  `json:"to,omitempty"`
	Lookback string `json:"lookback,omitempty"`
}

type NormalizedMetricsArgs struct {
	TargetName  string
	Expose      string
	Time        *nodeherder.TimeQuery
	Aggregation string
	Resolution  string
}

func MetricsToolSchema() proxy.Tool {
	return proxy.Tool{
		Type: "function",
		Function: proxy.FunctionSchema{
			Name:        "query_metrics",
			Description: "Query historical metrics for a specific device",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"target_name": map[string]any{
						"type":        "string",
						"description": "Natural language device name, e.g. 'garden', 'attic', 'living room'",
					},
					"expose": map[string]any{
						"type":        "string",
						"description": "Metric key ONLY from device.exposes.name. NEVER include units or symbols (e.g. 'temperature', not 'temperature (°C)')",
					},
					"time": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"from":     map[string]any{"type": "integer"},
							"to":       map[string]any{"type": "integer"},
							"lookback": map[string]any{"type": "string"},
						},
					},
					"aggregation": map[string]any{
						"type":        "string",
						"description": "Aggregation: last, min, max, avg, count",
					},
					"resolution": map[string]any{"type": "string"},
				},
				"required": []string{"target_name", "expose"},
			},
		},
	}
}

func ParseMetricsArgs(raw string) (MetricsArgs, error) {
	var args MetricsArgs
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return MetricsArgs{}, err
	}
	return args, nil
}

func NormalizeMetricsArgs(args MetricsArgs, cfg NormalizeConfig, clock Clock) (NormalizedMetricsArgs, error) {
	// Normalize tool args to protect downstream components from LLM artifacts.
	normalized := NormalizedMetricsArgs{
		TargetName: strings.TrimSpace(args.TargetName),
		Expose:     NormalizeExpose(args.Expose),
		Resolution: strings.TrimSpace(args.Resolution),
	}
	normalized.Aggregation = normalizeAggregation(args.Aggregation)

	if normalized.TargetName == "" || normalized.Expose == "" {
		return NormalizedMetricsArgs{}, fmt.Errorf("target_name and expose are required")
	}

	timeQuery := NormalizeTimeQuery(args.Time, cfg, clock)
	normalized.Time = timeQuery
	return normalized, nil
}

func NormalizeExpose(raw string) string {
	// Strip units/symbols and normalize to a safe key for device matching.
	s := strings.ToLower(strings.TrimSpace(raw))
	if idx := strings.Index(s, "("); idx >= 0 {
		s = s[:idx]
	}
	s = strings.TrimSpace(s)

	re := regexp.MustCompile(`[^a-z0-9_]+`)
	s = re.ReplaceAllString(s, " ")
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "_")
}

func NormalizeTimeQuery(args *TimeArgs, cfg NormalizeConfig, clock Clock) *nodeherder.TimeQuery {
	// Clamp and normalize time bounds to avoid negative or absurd ranges.
	if cfg.DefaultLookback == 0 {
		cfg = DefaultNormalizeConfig()
	}
	now := clock.Now().UTC()

	var fromMs, toMs int64
	var lookbackRaw string
	if args != nil {
		fromMs = args.From
		toMs = args.To
		lookbackRaw = args.Lookback
	}

	if fromMs < 0 {
		fromMs = 0
	}
	if toMs < 0 {
		toMs = 0
	}

	maxFuture := now.Add(cfg.MaxFutureSkew).UnixMilli()
	if fromMs > maxFuture {
		fromMs = 0
	}
	if toMs > maxFuture {
		toMs = 0
	}

	lookback, ok := NormalizeLookback(lookbackRaw, cfg)
	if !ok && fromMs == 0 && toMs == 0 {
		lookback = cfg.DefaultLookback
		ok = true
	}

	if ok {
		to := now
		from := to.Add(-lookback)
		return &nodeherder.TimeQuery{
			From: from,
			To:   to,
		}
	}

	q := &nodeherder.TimeQuery{}
	if fromMs != 0 {
		q.From = time.UnixMilli(fromMs).UTC()
	}
	if toMs != 0 {
		q.To = time.UnixMilli(toMs).UTC()
	}
	if !q.From.IsZero() && !q.To.IsZero() && q.To.Before(q.From) {
		q.From, q.To = q.To, q.From
	}
	return q
}

func NormalizeLookback(raw string, cfg NormalizeConfig) (time.Duration, bool) {
	// Normalize user/LLM lookbacks to bounded durations with safe defaults.
	if strings.TrimSpace(raw) == "" {
		return 0, false
	}
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "max" || s == "last" || s == "latest" {
		return 0, false
	}

	if d, ok := parseDurationTokens(s); ok {
		if d > cfg.MaxExplicitLookback {
			return cfg.MaxExplicitLookback, true
		}
		if d > cfg.MaxLookback {
			return d, true
		}
		return d, true
	}

	if d, err := time.ParseDuration(s); err == nil {
		if d > cfg.MaxExplicitLookback {
			return cfg.MaxExplicitLookback, true
		}
		if d > cfg.MaxLookback {
			return d, true
		}
		return d, true
	}

	return 0, false
}

func parseDurationTokens(s string) (time.Duration, bool) {
	re := regexp.MustCompile(`^(\d+)?\s*([a-zA-Z]+)$`)
	m := re.FindStringSubmatch(strings.ReplaceAll(s, " ", ""))
	if len(m) != 3 {
		return 0, false
	}

	n := 1
	if m[1] != "" {
		parsed, err := strconv.Atoi(m[1])
		if err != nil {
			return 0, false
		}
		n = parsed
	}

	unit := strings.ToLower(m[2])
	switch unit {
	case "minute", "minutes", "min", "mins", "m":
		return time.Duration(n) * time.Minute, true
	case "hour", "hours", "hr", "hrs", "h":
		return time.Duration(n) * time.Hour, true
	case "day", "days", "d":
		return time.Duration(n) * 24 * time.Hour, true
	case "week", "weeks", "w":
		return time.Duration(n) * 7 * 24 * time.Hour, true
	case "month", "months", "mo":
		return time.Duration(n) * 30 * 24 * time.Hour, true
	case "year", "years", "y":
		return time.Duration(n) * 365 * 24 * time.Hour, true
	default:
		return 0, false
	}
}

func normalizeAggregation(raw string) string {
	agg := strings.ToLower(strings.TrimSpace(raw))
	allowed := []string{
		string(nodeherder.AggLast),
		string(nodeherder.AggMin),
		string(nodeherder.AggMax),
		string(nodeherder.AggAvg),
		string(nodeherder.AggCount),
	}
	for _, v := range allowed {
		if agg == v {
			return agg
		}
	}
	return string(nodeherder.AggLast)
}

func BuildMaxLookbackTime(clock Clock, cfg NormalizeConfig) *nodeherder.TimeQuery {
	now := clock.Now().UTC()
	return &nodeherder.TimeQuery{
		From: now.Add(-cfg.MaxLookback),
		To:   now,
	}
}
