package proxy

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/kaptinlin/jsonrepair"
)

// Unified Tool Call Parser (Production Grade)
// This parser uses a tag-aware state machine to identify tool calls and 
// leverages the 'jsonrepair' library to robustly handle malformed LLM outputs.

var (
	// Tag definitions for identifying tool calls in a stream
	tagMarkers = []struct {
		namePattern *regexp.Regexp
		argStart    string
		argEnd      string
	}{
		// Standard XML: <function-name>NAME</function-name> <args-json-object>ARGS</args-json-object>
		{regexp.MustCompile(`(?s)<function-name>\s*(.*?)\s*</function-name>`), "<args-json-object>", "</args-json-object>"},
		// Call Tag (High Fidelity): <call:NAME>{"arg": "val"}</call:NAME>
		{regexp.MustCompile(`(?s)<call:([a-zA-Z0-9_-]+)>`), "", "</call:"},
		// Wrapper Tag: <tools>{"name":"...","arguments":{...}}</tools>
		{nil, "<tools>", "</tools>"},
		// Pipe Format: <|tool_call|>call:NAME{ARGS}<|tool_call|>
		{regexp.MustCompile(`(?s)<\|?tool_call\|?>(?:call:)?\s*([a-zA-Z0-9_]+)`), "{", ""},
		// Bracket Format: [TOOL_CALLS]NAME[ARGS]{ARGS}[TOOL_CALLS]
		{regexp.MustCompile(`(?s)\[TOOL_CALLS\]\s*([a-zA-Z0-9_]+)`), "[ARGS]", ""},
		// Native vLLM: functions.NAME:ID{ARGS}
		{regexp.MustCompile(`(?s)functions\.([a-zA-Z0-9_-]+):[a-zA-Z0-9_-]+`), "{", ""},
	}

	// Regex for Thought blocks (to be stripped from final content)
	reThoughtTag = regexp.MustCompile("(?s)<thought>.*?</thought>")

	// Regex for Markdown JSON blocks containing tool arrays
	reMarkdownJSON = regexp.MustCompile("(?s)```(?:json)?\n?\\s*(\\[\\s*\\{.*?\\}\\s*\\])\\s*\n?```")
	// Regex for raw JSON arrays (fallback for models that skip markdown blocks)
	reRawJSONArray = regexp.MustCompile("(?s)(\\[\\s*\\{\\s*\"(?:tool|function|name)\".*?\\}\\s*\\])")
	// Pattern for custom string markers <|"|>...<|"|> (Gemma-specific)
	reCustomStringMarker = regexp.MustCompile(`(?s)<\|"\|>(.*?)<\|"\|>`)
	// Pattern for any XML tag (for final content cleanup)
	reAnyXMLTag = regexp.MustCompile(`<[^>]*>`)
)

// ParseContentToolCalls is the entry point for extracting tool calls from LLM content.
func ParseContentToolCalls(content string) (string, []ToolCall, bool) {
	var calls []ToolCall

	// 1. Try Markdown JSON blocks first (high precision)
	if match := reMarkdownJSON.FindStringSubmatch(content); len(match) > 0 {
		if _, tc, ok := parseJSONArray(match[1]); ok {
			cleaned := reMarkdownJSON.ReplaceAllString(content, "")
			return cleaned, tc, true
		}
	}

	// 1b. Try Raw JSON arrays (fallback for models that skip markdown blocks)
	if match := reRawJSONArray.FindStringSubmatch(content); len(match) > 0 {
		if _, tc, ok := parseJSONArray(match[1]); ok {
			cleaned := reRawJSONArray.ReplaceAllString(content, "")
			return cleaned, tc, true
		}
	}

	// 2. Format-agnostic tag scanning
	for _, marker := range tagMarkers {
		if marker.namePattern != nil {
			matches := marker.namePattern.FindAllStringSubmatchIndex(content, -1)
			for _, match := range matches {
				name := strings.TrimSpace(content[match[2]:match[3]])
				
				// Extract args starting after the name match
				argContent, found := extractBalancedJSON(content, match[1], marker.argStart, marker.argEnd)
				if found {
					repaired, err := jsonrepair.Repair(standardizeCustomMarkers(argContent))
					if err == nil {
						calls = append(calls, ToolCall{
							ID:   fmt.Sprintf("call-%d", len(calls)),
							Type: "function",
							Function: FunctionCall{
								Name:      name,
								Arguments: repaired,
							},
						})
					}
				}
			}
		} else {
			// Handle Wrapper tags like <tools>
			startIdx := 0
			for {
				loc := strings.Index(content[startIdx:], marker.argStart)
				if loc == -1 {
					break
				}
				absStart := startIdx + loc
				argContent, found := extractBalancedJSON(content, absStart, marker.argStart, marker.argEnd)
				if found {
					var wrapper struct {
						Name      string          `json:"name"`
						Arguments json.RawMessage `json:"arguments"`
					}
					repaired, err := jsonrepair.Repair(standardizeCustomMarkers(argContent))
					if err == nil && json.Unmarshal([]byte(repaired), &wrapper) == nil && wrapper.Name != "" {
						calls = append(calls, ToolCall{
							ID:   fmt.Sprintf("wrap-%d", len(calls)),
							Type: "function",
							Function: FunctionCall{
								Name:      wrapper.Name,
								Arguments: string(wrapper.Arguments),
							},
						})
					}
				}
				startIdx = absStart + len(marker.argStart)
			}
		}
	}

	// 3. Fallback: Raw JSON object (if no tags found)
	if len(calls) == 0 {
		trimmed := strings.TrimSpace(content)
		if strings.HasPrefix(trimmed, "{") {
			var rc map[string]json.RawMessage
			repaired, err := jsonrepair.Repair(standardizeCustomMarkers(trimmed))
			if err == nil && json.Unmarshal([]byte(repaired), &rc) == nil {
				name := ""
				if val, ok := rc["tool"]; ok {
					json.Unmarshal(val, &name)
				} else if val, ok := rc["function"]; ok {
					json.Unmarshal(val, &name)
				} else if val, ok := rc["name"]; ok {
					json.Unmarshal(val, &name)
				}

				var args json.RawMessage
				if val, ok := rc["args"]; ok {
					args = val
				} else if val, ok := rc["parameters"]; ok {
					args = val
				} else if val, ok := rc["arguments"]; ok {
					args = val
				}

				if name != "" {
					calls = append(calls, ToolCall{
						ID:   "raw-0",
						Type: "function",
						Function: FunctionCall{
							Name:      name,
							Arguments: string(args),
						},
					})
					return "", calls, true
				}
			}
		}
	}

	// 4. Cleanup: Only remove the specific substrings that were successfully parsed as tool calls.
	// This ensures that if a tool call was malformed, the tags remain in the content for debugging.
	cleaned := reThoughtTag.ReplaceAllString(content, "")
	if len(calls) > 0 {
		for _, marker := range tagMarkers {
			if marker.namePattern != nil {
				cleaned = marker.namePattern.ReplaceAllString(cleaned, "")
			}
			if marker.argStart != "" {
				cleaned = strings.ReplaceAll(cleaned, marker.argStart, "")
			}
			if marker.argEnd != "" {
				cleaned = strings.ReplaceAll(cleaned, marker.argEnd, "")
			}
		}
		cleaned = reAnyXMLTag.ReplaceAllString(cleaned, "")
	}
	cleaned = strings.TrimSpace(cleaned)

	if len(calls) == 0 {
		return content, nil, false
	}

	return cleaned, calls, true
}

// extractBalancedJSON finds the first JSON block and uses a brace counter to extract it.
func extractBalancedJSON(content string, startOffset int, markerStart, markerEnd string) (string, bool) {
	if startOffset >= len(content) {
		return "", false
	}

	sub := content[startOffset:]
	var actualStart int
	if markerStart != "" {
		idx := strings.Index(sub, markerStart)
		if idx == -1 {
			return "", false
		}
		if markerStart == "{" {
			actualStart = idx
		} else {
			actualStart = idx + len(markerStart)
		}
	}
	
	jsonStart := strings.Index(sub[actualStart:], "{")
	if jsonStart == -1 {
		return "", false
	}
	jsonStart += actualStart

	braceCount := 0
	inString := false
	isEscaped := false
	
	for i := jsonStart; i < len(sub); i++ {
		char := sub[i]
		if isEscaped {
			isEscaped = false
			continue
		}
		if char == '\\' {
			isEscaped = true
			continue
		}
		if char == '"' {
			inString = !inString
			continue
		}
		if !inString {
			if char == '{' {
				braceCount++
			} else if char == '}' {
				braceCount--
				if braceCount == 0 {
					return sub[jsonStart : i+1], true
				}
			}
		}
		if markerEnd != "" && strings.HasPrefix(sub[i:], markerEnd) {
			return sub[jsonStart:i], true
		}
	}

	return sub[jsonStart:], true
}

// standardizeCustomMarkers handles Gemma-style <|"|> markers before jsonrepair takes over.
func standardizeCustomMarkers(input string) string {
	return reCustomStringMarker.ReplaceAllStringFunc(input, func(match string) string {
		inner := reCustomStringMarker.FindStringSubmatch(match)[1]
		if inner == "true" || inner == "false" {
			return inner
		}
		if _, err := strconv.ParseFloat(inner, 64); err == nil {
			if len(inner) > 1 && inner[0] == '0' && inner[1] != '.' {
				b, _ := json.Marshal(inner)
				return string(b)
			}
			return inner
		}
		b, _ := json.Marshal(inner)
		return string(b)
	})
}

// parseJSONArray extracts tool calls from a JSON array string.
func parseJSONArray(input string) (string, []ToolCall, bool) {
	var rawArray []map[string]json.RawMessage
	repaired, err := jsonrepair.Repair(input)
	if err != nil {
		return "", nil, false
	}
	if err := json.Unmarshal([]byte(repaired), &rawArray); err == nil {
		var calls []ToolCall
		for i, rc := range rawArray {
			name := ""
			if val, ok := rc["tool"]; ok {
				json.Unmarshal(val, &name)
			} else if val, ok := rc["function"]; ok {
				json.Unmarshal(val, &name)
			} else if val, ok := rc["name"]; ok {
				json.Unmarshal(val, &name)
			}

			var args json.RawMessage
			if val, ok := rc["args"]; ok {
				args = val
			} else if val, ok := rc["parameters"]; ok {
				args = val
			} else if val, ok := rc["arguments"]; ok {
				args = val
			}

			if name != "" {
				calls = append(calls, ToolCall{
					ID:   fmt.Sprintf("tc-%d", i),
					Type: "function",
					Function: FunctionCall{
						Name:      name,
						Arguments: string(args),
					},
				})
			}
		}

		if len(calls) > 0 {
			return "", calls, true
		}
	}

	return "", nil, false
}
