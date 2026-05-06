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
	// Regex for Thought blocks (to be stripped from final content)
	reThoughtTag = regexp.MustCompile("(?s)<thought>.*?</thought>")

	// Regex for Markdown JSON blocks containing tool arrays
	reMarkdownJSON = regexp.MustCompile("(?s)```(?:json)?\n?\\s*(\\[\\s*\\{.*?\\}\\s*\\])\\s*\n?```")
	// Regex for raw JSON arrays (fallback for models that skip markdown blocks)
	reRawJSONArray = regexp.MustCompile("(?s)(\\[\\s*\\{\\s*\"(?:tool|function|name)\".*?\\}\\s*\\])")
	// Regex for the new <tool_call> tag format
	reToolCallTag = regexp.MustCompile("(?s)<tool_call>(.*?)</tool_call>")
	// Pattern for custom string markers <|"|>...<|"|> (Gemma-specific)
	reCustomStringMarker = regexp.MustCompile(`(?s)<\|"\|>(.*?)<\|"\|>`)
)

// ParseContentToolCalls is the entry point for extracting tool calls from LLM content.
func ParseContentToolCalls(content string) (string, []ToolCall, bool) {
	var calls []ToolCall

	// 1. Priority 1: Search for <tool_call> tags (Universal Agnostic Standard)
	// We use a lenient approach that handles missing closing tags.
	openingTag := "<tool_call>"
	idx := strings.Index(content, openingTag)
	if idx != -1 {
		var raw string
		closingTag := "</tool_call>"
		endIdx := strings.Index(content[idx:], closingTag)

		if endIdx != -1 {
			// Perfect match with closing tag
			raw = content[idx+len(openingTag) : idx+endIdx]
		} else {
			// Lenient match: extract balanced JSON starting after the opening tag
			jsonStr, ok := extractBalancedJSON(content, idx+len(openingTag), "{", "}")
			if ok {
				raw = jsonStr
			}
		}

		if raw != "" {
			// The tag content can be a single tool object or an array of tools
			trimmedRaw := strings.TrimSpace(raw)
			if strings.HasPrefix(trimmedRaw, "[") {
				if _, tc, ok := parseJSONArray(trimmedRaw); ok {
					cleaned := strings.Replace(content, openingTag, "", -1)
					if endIdx != -1 {
						cleaned = strings.Replace(cleaned, closingTag, "", -1)
					}
					// Also remove the raw JSON from content to avoid duplication
					cleaned = strings.Replace(cleaned, trimmedRaw, "", -1)
					return cleaned, tc, true
				}
			} else {
				// Single tool object
				if _, tc, ok := parseJSONObject(trimmedRaw); ok {
					cleaned := strings.Replace(content, openingTag, "", -1)
					if endIdx != -1 {
						cleaned = strings.Replace(cleaned, closingTag, "", -1)
					}
					cleaned = strings.Replace(cleaned, trimmedRaw, "", -1)
					return cleaned, tc, true
				}
			}
		}

		// If we found a tag but failed to parse it, return a helpful error
		errorCall := ToolCall{
			ID:   "parse-error",
			Type: "function",
			Function: FunctionCall{
				Name: "system_error",
				Arguments: fmt.Sprintf(`{"error": "SYSTEM ERROR: Malformed tool call inside <tool_call> tags. Ensure your JSON is valid. raw content: %s"}`, 
					strings.ReplaceAll(strings.ReplaceAll(raw, `"`, `\"`), "\n", "\\n")),
			},
		}
		return content, []ToolCall{errorCall}, true
	}

	// 2. Priority 2: Try Markdown JSON blocks (high precision)
	if match := reMarkdownJSON.FindStringSubmatch(content); len(match) > 0 {
		if _, tc, ok := parseJSONArray(match[1]); ok {
			cleaned := reMarkdownJSON.ReplaceAllString(content, "")
			return cleaned, tc, true
		}
	}

	// 3. Priority 3: Try Raw JSON arrays
	if match := reRawJSONArray.FindStringSubmatch(content); len(match) > 0 {
		if _, tc, ok := parseJSONArray(match[1]); ok {
			cleaned := reRawJSONArray.ReplaceAllString(content, "")
			return cleaned, tc, true
		}
	}

	// 4. Priority 4: Try Custom String Markers (e.g. Gemma <|"|>)
	if match := reCustomStringMarker.FindStringSubmatch(content); len(match) > 0 {
		if _, tc, ok := parseJSONObject(match[1]); ok {
			cleaned := reCustomStringMarker.ReplaceAllString(content, "")
			return cleaned, tc, true
		}
	}

	// 4b. Priority 4b: Generic "Name{Args}" Scanner (Agnostic Fallback)
	// Handles: tool_name{"key": "value"} or call:tool_name{...}
	// This is common for models that struggle with wrapping the name inside JSON.
	nameIdx := strings.Index(content, "{")
	if nameIdx != -1 {
		// Look backwards from the first '{' to find the start of the tool name.
		// We accept letters, numbers, underscores, dots, and colons (for 'call:').
		start := nameIdx
		for start > 0 {
			char := content[start-1]
			if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' || char == '.' || char == ':' {
				start--
			} else {
				break
			}
		}
		rawName := content[start:nameIdx]
		// Clean name (remove "call:", ":", etc.)
		toolName := strings.Trim(rawName, " :")
		if strings.HasPrefix(toolName, "call:") {
			toolName = toolName[5:]
		}

		// Extract balanced JSON starting at nameIdx
		if argsStr, ok := extractBalancedJSON(content, nameIdx, "{", "}"); ok {
			// Verify if it looks like a tool name (no weird characters and reasonable length)
			if len(toolName) > 2 && len(toolName) < 64 {
				// Standardize markers and repair JSON in arguments
				repairedArgs := standardizeCustomMarkers(argsStr)
				if repaired, err := jsonrepair.Repair(repairedArgs); err == nil {
					repairedArgs = repaired
				}

				tc := ToolCall{
					ID:   "agnostic-call",
					Type: "function",
					Function: FunctionCall{
						Name:      toolName,
						Arguments: repairedArgs,
					},
				}
				// Verify if it's a known tool or likely action
				// (We are very lenient here to support agnostic discovery)
				cleaned := strings.Replace(content, rawName, "", 1)
				cleaned = strings.Replace(cleaned, argsStr, "", 1)
				return cleaned, []ToolCall{tc}, true
			}
		}
	}

	// 5. Priority 5: Global Balanced JSON Scanner (The Ultimate Fallback)
	// This searches the entire content for the first '{' and tries to extract a valid tool call.
	// This handles models that ignore tags entirely and just output JSON mixed with text.
	if jsonStr, ok := extractBalancedJSON(content, 0, "{", "}"); ok {
		if _, tc, ok := parseJSONObject(jsonStr); ok {
			cleaned := strings.Replace(content, jsonStr, "", 1)
			return cleaned, tc, true
		}
	}

	// Cleanup and return
	cleaned := strings.TrimSpace(content)

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
// parseJSONObject extracts a single tool call from a JSON object string.
func parseJSONObject(input string) (string, []ToolCall, bool) {
	var rc map[string]json.RawMessage
	repaired, err := jsonrepair.Repair(standardizeCustomMarkers(input))
	if err != nil {
		return "", nil, false
	}
	if err := json.Unmarshal([]byte(repaired), &rc); err != nil {
		return "", nil, false
	}

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
		return "", []ToolCall{
			{
				ID:   "tc-0",
				Type: "function",
				Function: FunctionCall{
					Name:      name,
					Arguments: string(args),
				},
			},
		}, true
	}

	return "", nil, false
}
