package tools

import (
	"llm-proxy/internal/nodeherder"
	"strings"
)

// ExtractMentionedMetrics returns the list of metrics that appear in the user message.
// It checks against all available exposes from the device context for a dynamic solution.
func ExtractMentionedMetrics(userMessage string, deviceCtx *nodeherder.LLMDeviceContext) []string {
	message := strings.ToLower(userMessage)

	// Build set of all unique expose names from device context
	availableMetrics := make(map[string]bool)
	for _, device := range deviceCtx.Devices {
		for _, expose := range device.Exposes {
			availableMetrics[strings.ToLower(expose.Name)] = true
		}
	}

	// Common synonyms/abbreviations that map to actual metric names
	synonyms := map[string]string{
		"temp":        "temperature",
		"humid":       "humidity",
		"motion":      "occupancy",
		"movement":    "occupancy",
		"open":        "contact",
		"closed":      "contact",
		"door":        "contact",
		"window":      "contact",
		"battery":     "battery",
		"power":       "power",
		"energy":      "energy",
		"voltage":     "voltage",
		"co2":         "co2",
		"carbon":      "co2",
		"pressure":    "pressure",
		"illuminance": "illuminance",
		"light":       "illuminance",
		"brightness":  "brightness",
	}

	mentioned := make(map[string]bool)

	// Check for direct metric mentions
	for metric := range availableMetrics {
		if strings.Contains(message, metric) {
			mentioned[metric] = true
		}
	}

	// Check for synonym mentions
	for synonym, metric := range synonyms {
		if strings.Contains(message, synonym) && availableMetrics[metric] {
			mentioned[metric] = true
		}
	}

	// Convert to slice
	result := make([]string, 0, len(mentioned))
	for metric := range mentioned {
		result = append(result, metric)
	}

	return result
}

// FilterMetricsByMentioned filters a list of requested metrics to only include
// those that were actually mentioned in the user message.
// If no metrics were detected in the message, returns the original list (fallback).
func FilterMetricsByMentioned(requested []string, mentioned []string) []string {
	if len(mentioned) == 0 {
		// No metrics detected - fallback to original request
		return requested
	}

	mentionedSet := make(map[string]bool)
	for _, m := range mentioned {
		mentionedSet[strings.ToLower(m)] = true
	}

	filtered := make([]string, 0, len(requested))
	for _, req := range requested {
		if mentionedSet[strings.ToLower(req)] {
			filtered = append(filtered, req)
		}
	}

	if len(filtered) == 0 {
		// No overlap - fallback to original request
		return requested
	}

	return filtered
}
