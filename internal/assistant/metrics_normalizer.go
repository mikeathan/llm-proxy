package assistant

import (
	"fmt"
	"llm-proxy/internal/nodeherder"
	"time"
)

type MetricResult struct {
	Metric      string
	DeviceName  string // Human-readable device name for correct attribution
	DeviceID    string
	Operation   string
	Value       any
	From        time.Time
	To          time.Time
	Timestamp   *time.Time `json:",omitempty"`
	LastChanged *time.Time `json:",omitempty"`
	Note        string     `json:",omitempty"` // Important context about the data
}

func NormalizeMetrics(resp *nodeherder.MetricsQueryResponse, aggregation nodeherder.AggregationType, lookbackExpanded bool, deviceName string, expose *nodeherder.LLMExpose) MetricResult {

	if len(resp.Values) == 0 {
		return MetricResult{
			Metric:     resp.Expose,
			DeviceName: deviceName,
			Operation:  string(aggregation),
			From:       time.UnixMilli(resp.From),
			To:         time.UnixMilli(resp.To),
		}
	}

	v := resp.Values[0]

	if v.Value == nil {
		return MetricResult{
			Metric:     resp.Expose,
			DeviceName: deviceName,
			DeviceID:   v.DeviceId,
			Operation:  string(aggregation),
			From:       time.UnixMilli(resp.From),
			To:         time.UnixMilli(resp.To),
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
	} else if expose != nil && expose.Type == "binary" {
		// Translate binary values to human-readable using expose metadata
		resultValue = translateBinaryValue(v.Value, expose, resp.Expose)
	}

	result := MetricResult{
		Metric:     resp.Expose,
		DeviceName: deviceName,
		DeviceID:   v.DeviceId,
		Operation:  string(aggregation),
		Value:      resultValue,
		From:       time.UnixMilli(resp.From),
		To:         time.UnixMilli(resp.To),
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

	if lookbackExpanded && ts != nil {
		// Don't return the Value when lookback is expanded - only the Note.
		// This prevents the LLM from misreporting old data as "today's value".
		return MetricResult{
			Metric:     resp.Expose,
			DeviceName: deviceName,
			DeviceID:   v.DeviceId,
			Operation:  string(aggregation),
			From:       time.UnixMilli(resp.From),
			To:         time.UnixMilli(resp.To),
			Note: fmt.Sprintf("No %s data for today (%s). Last known: %v from %s",
				resp.Expose, deviceName, v.Value, ts.Format("Jan 2, 2006")),
		}
	}

	return result
}

// translateBinaryValue converts a binary sensor value to human-readable text
// using the expose's valueOn/valueOff mapping.
func translateBinaryValue(value any, expose *nodeherder.LLMExpose, exposeName string) string {
	if expose == nil {
		return fmt.Sprintf("%v", value)
	}

	// Check if value matches valueOn
	if valuesEqual(value, expose.On) {
		return binaryOnLabel(exposeName)
	}
	// Check if value matches valueOff
	if valuesEqual(value, expose.Off) {
		return binaryOffLabel(exposeName)
	}

	// Fallback to raw value
	return fmt.Sprintf("%v", value)
}

// valuesEqual compares two values that may be of different types
func valuesEqual(a, b any) bool {
	if a == nil || b == nil {
		return a == b
	}
	// Handle bool comparison with JSON decoded values (which may be float64)
	switch av := a.(type) {
	case bool:
		switch bv := b.(type) {
		case bool:
			return av == bv
		}
	case float64:
		switch bv := b.(type) {
		case float64:
			return av == bv
		case bool:
			// false = 0, true = 1
			return (av == 0 && !bv) || (av == 1 && bv)
		}
	case string:
		switch bv := b.(type) {
		case string:
			return av == bv
		}
	}
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

// binaryOnLabel returns human-readable label for the "on" state based on expose type
func binaryOnLabel(exposeName string) string {
	switch exposeName {
	case "contact":
		return "open" // Contact sensor "on" = contact broken = door OPEN
	case "presence", "occupancy":
		return "presence detected"
	case "state":
		return "on"
	case "smoke", "alarm":
		return "triggered"
	default:
		return "on"
	}
}

// binaryOffLabel returns human-readable label for the "off" state based on expose type
func binaryOffLabel(exposeName string) string {
	switch exposeName {
	case "contact":
		return "closed" // Contact sensor "off" = contact made = door CLOSED
	case "presence", "occupancy":
		return "no presence"
	case "state":
		return "off"
	case "smoke", "alarm":
		return "normal"
	default:
		return "off"
	}
}
