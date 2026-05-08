package proxy

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/kaptinlin/jsonrepair"
)

var xmlToolRegex = regexp.MustCompile(`(?is)<tool_call>\s*(.*?)\s*</tool_call>`)

// unquotedKeyRegex: Fixes tool:mkdir or {tool:mkdir}.
// Standard Go \s doesn't catch Unicode non-breaking spaces (\x{00A0}).
var unquotedKeyRegex = regexp.MustCompile(`(?m)(^|[{,][\s\x{00A0}]*)([a-zA-Z0-9_]+)([\s\x{00A0}]*:)`)

// keyGarbageRegex: Strips leading dots/dashes from keys like { .tool: or { -args:
var keyGarbageRegex = regexp.MustCompile(`(?m)(^|[{,][\s\x{00A0}]*)[^a-zA-Z0-9_"]+([a-zA-Z0-9_]+[\s\x{00A0}]*:)`)

var trailingCommaRegex = regexp.MustCompile(`,\s*([}\]])`)

func sanitizeJSON(input string) string {
	// 1. Discovery: Find the first '{' that likely starts our tool call.
	// We anchor to "tool" to avoid conversational braces.
	toolIdx := strings.Index(input, "\"tool\":")
	if toolIdx == -1 {
		toolIdx = strings.Index(input, "tool:")
	}
	
	start := 0
	if toolIdx != -1 {
		start = strings.LastIndex(input[:toolIdx], "{")
		if start == -1 {
			start = 0
		}
	} else {
		start = strings.Index(input, "{")
		if start == -1 {
			start = 0
		}
	}

	// 2. Extract the likely JSON segment
	rawSegment := input[start:]
	
	// 3. Use the professional jsonrepair library to fix everything.
	// This handles unquoted keys, literal newlines, trailing commas, and missing braces.
	repaired, err := jsonrepair.Repair(rawSegment)
	if err != nil {
		// Fallback: If repair fails, return original to let the unmarshaler log the error
		return rawSegment
	}

	return repaired
}

func ParseContentToolCalls(content string) (string, []ToolCall, bool) {
	var allCalls []ToolCall
	cleanedContent := content

	// Phase 1: Fuzzy XML/Tag detection
	// We look for anything that starts with <tool and ends with any tag-like structure
	reFuzzyTags := regexp.MustCompile(`(?is)<tool(?:_call)?>\s*(.*?)\s*</?tool(?:_call)?>`)
	matches := reFuzzyTags.FindAllStringSubmatch(content, -1)

	if len(matches) > 0 {
		for i, match := range matches {
			fullMatch := match[0]
			jsonStr := sanitizeJSON(match[1])
			if call, ok := parseJSONToolCall(jsonStr, i); ok {
				allCalls = append(allCalls, call)
				cleanedContent = strings.Replace(cleanedContent, fullMatch, "", 1)
			}
		}
		if len(allCalls) > 0 {
			return strings.TrimSpace(cleanedContent), allCalls, true
		}
	}

	// Phase 2: Greedy Fallback (Naked JSON detection)
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start != -1 && end != -1 && end > start {
		jsonStr := sanitizeJSON(content[start : end+1])
		if call, ok := parseJSONToolCall(jsonStr, 0); ok {
			cleaned := content[:start] + content[end+1:]
			return strings.TrimSpace(cleaned), []ToolCall{call}, true
		}
	}

	// Phase 3: Ultra-Greedy Fallback (Naked Key-Value detection for broken JSON)
	if strings.Contains(content, "\"tool\":") && strings.Contains(content, "\"args\":") {
		// Attempt to wrap the whole content (or a suspicious slice) in braces
		jsonStr := sanitizeJSON(content)
		if !strings.HasPrefix(jsonStr, "{") {
			jsonStr = "{" + jsonStr + "}"
		}
		if call, ok := parseJSONToolCall(jsonStr, 0); ok {
			return "", []ToolCall{call}, true
		}
	}

	return content, nil, false
}

func parseJSONToolCall(jsonStr string, index int) (ToolCall, bool) {
	var call struct {
		Tool string          `json:"tool"`
		Args json.RawMessage `json:"args"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &call); err != nil {
		// DIAGNOSTIC: Print the error to terminal for the developer
		fmt.Printf("\n[PARSER ERROR] JSON unmarshal failed: %v\nRAW STRING: %s\n", err, jsonStr)
		return ToolCall{}, false
	}

	if call.Tool == "" {
		return ToolCall{}, false
	}

	return ToolCall{
		ID:       fmt.Sprintf("tc-%d", index),
		Type:     "function",
		Function: FunctionCall{Name: call.Tool, Arguments: string(call.Args)},
	}, true
}
