package assistant

import (
	"llm-proxy/internal/nodeherder"
	"time"
)

type MetricResult struct {
	Metric      string
	DeviceID    string
	Operation   string
	Value       any
	From        time.Time
	To          time.Time
	Timestamp   *time.Time `json:",omitempty"`
	LastChanged *time.Time `json:",omitempty"`
}

func NormalizeMetrics(resp *nodeherder.MetricsQueryResponse, aggregation nodeherder.AggregationType) MetricResult {

	if len(resp.Values) == 0 {
		return MetricResult{
			Metric:    resp.Expose,
			Operation: string(aggregation),
			From:      time.UnixMilli(resp.From),
			To:        time.UnixMilli(resp.To),
		}
	}

	v := resp.Values[0]

	if v.Value == nil {
		return MetricResult{
			Metric:    resp.Expose,
			DeviceID:  v.DeviceId,
			Operation: string(aggregation),
			From:      time.UnixMilli(resp.From),
			To:        time.UnixMilli(resp.To),
		}
	}

	var ts *time.Time
	if v.Timestamp > 0 {
		t := time.UnixMilli(v.Timestamp)
		ts = &t
	}

	var resultValue any = v.Value
	if aggregation == nodeherder.LastEvent || aggregation == "event" {
		// For event queries, the value found IS the match.
		// If the specific device value is false (e.g. Inverted sensor), returning "false" confuses the LLM
		// into thinking it found the "Closed" state instead of the "Open" (false) state it asked for.
		// We set it to a semantic confirmation instead.
		resultValue = "Event Found"
	}

	result := MetricResult{
		Metric:    resp.Expose,
		DeviceID:  v.DeviceId,
		Operation: string(aggregation),
		Value:     resultValue,
		From:      time.UnixMilli(resp.From),
		To:        time.UnixMilli(resp.To),
	}

	// Use specific field names to help the LLM understand the temporal context
	// of the result, especially for "last value" queries vs statistical aggregates.
	isLast := aggregation == nodeherder.AggLast ||
		aggregation == nodeherder.LastEvent ||
		aggregation == "event"

	if isLast {
		result.LastChanged = ts
	} else {
		result.Timestamp = ts
	}

	return result
}
