package assistant

import (
	"llm-proxy/internal/nodeherder"
	"time"
)

type MetricResult struct {
	Metric    string
	DeviceID  string
	Operation string
	Value     any
	From      time.Time
	To        time.Time
	Timestamp *time.Time
}

func NormalizeMetrics(resp *nodeherder.MetricsQueryResponse, aggregation nodeherder.AggregationType) MetricResult {
	var ts *time.Time

	switch aggregation {
	case nodeherder.AggLast, nodeherder.AggMin, nodeherder.AggMax:
		t := time.UnixMilli(resp.Values[0].Timestamp)
		ts = &t
	}

	return MetricResult{
		Metric:    resp.Expose,
		DeviceID:  resp.Values[0].DeviceId,
		Operation: string(aggregation),
		Value:     resp.Values[0].Value,
		From:      time.UnixMilli(resp.From),
		To:        time.UnixMilli(resp.To),
		Timestamp: ts,
	}
}
