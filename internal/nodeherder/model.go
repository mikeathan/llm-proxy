package nodeherder

import "time"

// Device context response structure
type AggregationType string

const (
	AggNone  AggregationType = ""
	AggLast  AggregationType = "last"
	AggMin   AggregationType = "min"
	AggMax   AggregationType = "max"
	AggAvg   AggregationType = "avg"
	AggCount AggregationType = "count"
	LastEvent AggregationType = "last_event"
)

type DeviceContextResponse struct {
	Version     string          `json:"version"`
	GeneratedAt time.Time       `json:"generatedAt"`
	Devices     []DeviceContext `json:"devices"`
}

type DeviceContext struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Exposes     []ExposeInfo `json:"exposes"`
}

type ExposeInfo struct {
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	Unit         string            `json:"unit,omitempty"`
	Values       []string          `json:"values,omitempty"`
	ValueOn      any               `json:"valueOn,omitempty"`
	ValueOff     any               `json:"valueOff,omitempty"`
	ValueToggle  any               `json:"valueToggle,omitempty"`
	Aggregations []AggregationType `json:"aggregations"`
}

type TimeQuery struct {
	From     time.Time `json:"from,omitempty"`
	To       time.Time `json:"to,omitempty"`
	Lookback string    `json:"lookback,omitempty"`
}

// Query Request
type MetricsQueryRequest struct {
	DeviceIDs        []string        `json:"deviceIds"`
	Expose           string          `json:"expose"`
	Time             *TimeQuery      `json:"time,omitempty"`
	Aggregation      AggregationType `json:"aggregation"`
	AggregationValue any             `json:"aggregationValue,omitempty"`
}

// Query Response
type MetricsQueryResponse struct {
	Expose string
	From   int64
	To     int64
	Values []MetricsQueryDeviceResponse
}

type MetricsQueryDeviceResponse struct {
	DeviceId  string `json:"deviceId"`
	Value     any    `json:"value"`
	Timestamp int64  `json:"timestamp,omitempty"`
}
