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

// DetectMultipleDevices checks if the user message mentions multiple distinct devices.
// Returns the list of matched device names if more than one device is mentioned.
// Uses device context to dynamically determine which words are generic (expose names, common words).
func DetectMultipleDevices(userMessage string, deviceCtx *nodeherder.LLMDeviceContext) []string {
	message := strings.ToLower(userMessage)

	// Build set of all expose names (these are generic and shouldn't trigger device matching)
	genericWords := make(map[string]bool)
	for _, device := range deviceCtx.Devices {
		for _, expose := range device.Exposes {
			// Add expose name and its parts as generic
			exposeLower := strings.ToLower(expose.Name)
			genericWords[exposeLower] = true
			for _, part := range strings.Fields(exposeLower) {
				genericWords[part] = true
			}
		}
	}
	// Add common English words that shouldn't trigger matching
	for _, w := range []string{"the", "a", "an", "and", "or", "in", "on", "at", "to", "for", "is", "was", "what", "when", "how", "sensor", "device"} {
		genericWords[w] = true
	}

	matchedDevices := make([]string, 0)

	for _, device := range deviceCtx.Devices {
		deviceNameLower := strings.ToLower(device.Name)

		// Extract distinctive tokens from device name (not generic words)
		tokens := strings.Fields(deviceNameLower)
		for _, token := range tokens {
			// Skip if it's a generic word (expose name or common word)
			if genericWords[token] {
				continue
			}
			// If this distinctive token (3+ chars) appears in the message, device is mentioned
			if len(token) >= 3 && strings.Contains(message, token) {
				matchedDevices = append(matchedDevices, device.Name)
				break
			}
		}
	}

	// Only return if multiple devices detected
	if len(matchedDevices) > 1 {
		return matchedDevices
	}
	return nil
}
