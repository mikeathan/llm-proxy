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

	if len(resp.Values) == 0 {
		return MetricResult{
			Metric:    resp.Expose,
			Operation: string(aggregation),
			From:      time.UnixMilli(resp.From),
			To:        time.UnixMilli(resp.To),
		}
	}

	v := resp.Values[0]

	var ts *time.Time
	switch aggregation {
	case nodeherder.AggLast, nodeherder.AggMin, nodeherder.AggMax:
		if v.Timestamp > 0 {
			t := time.UnixMilli(v.Timestamp)
			ts = &t
		}
	}

	return MetricResult{
		Metric:    resp.Expose,
		DeviceID:  v.DeviceId,
		Operation: string(aggregation),
		Value:     v.Value,
		From:      time.UnixMilli(resp.From),
		To:        time.UnixMilli(resp.To),
		Timestamp: ts,
	}
}
