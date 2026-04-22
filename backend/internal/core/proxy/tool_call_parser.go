package proxy

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// These patterns match the two XML-like tool call formats used by some local
// models (e.g. qwen2.5-coder) instead of the standard OpenAI tool_calls field.
//
// Format 1 – function-name / args-json-object tags:
//
//	<function-name>query_device</function-name>
//	<args-json-object>{"target_name": "Living Room Light"}</args-json-object>
//
// Format 2 – tools tag wrapping a JSON object:
//
//	<tools>
//	{"name": "query_device", "arguments": {"target_name": "Living Room Light"}}
//	</tools>
var (
	reToolName  = regexp.MustCompile(`(?s)<function-name>(.*?)</function-name>`)
	reToolArgs  = regexp.MustCompile(`(?s)<args-json-object>(.*?)</args-json-object>`)
	reToolsTag  = regexp.MustCompile(`(?s)<tools>(.*?)</tools>`)
	// Match functions.<name>:<id>{args} (common in vLLM/OpenAI-like local outputs)
	reNativeFunc = regexp.MustCompile(`(?s)functions\.([a-zA-Z0-9_-]+):([a-zA-Z0-9_-]+)(\{.*?\})`)
	// Match markdown JSON blocks containing tool call arrays
	reMarkdownJSON = regexp.MustCompile("(?s)```(?:json)?\n\\s*(\\[\\s*\\{.*?\\}\\s*\\])\\s*\n```")
	// Match raw JSON objects (often used by models that ignore formatting instructions)
	reRawJSON = regexp.MustCompile(`(?s)^\s*(\{.*?\})\s*$`)
)

// ParseContentToolCalls inspects a message's Content for embedded tool call
// markup. If found, it returns a synthesised ToolCall slice and true, along
// with a cleaned version of the content that strips the tool call markup.
func ParseContentToolCalls(content string) (string, []ToolCall, bool) {
	// Try format 1: <function-name> / <args-json-object>
	if calls, ok := parseFunctionNameFormat(content); ok {
		cleaned := reToolName.ReplaceAllString(content, "")
		cleaned = reToolArgs.ReplaceAllString(cleaned, "")
		return cleaned, calls, true
	}
	// Try format 2: <tools> wrapping a JSON object
	if calls, ok := parseToolsTagFormat(content); ok {
		cleaned := reToolsTag.ReplaceAllString(content, "")
		return cleaned, calls, true
	}
	// Try format 3: functions.<name>:<id>{args}
	if calls, ok := parseNativeFuncFormat(content); ok {
		cleaned := reNativeFunc.ReplaceAllString(content, "")
		return cleaned, calls, true
	}
	// Try format 4: Markdown JSON blocks
	if calls, ok := parseMarkdownJSONFormat(content); ok {
		cleaned := reMarkdownJSON.ReplaceAllString(content, "")
		return cleaned, calls, true
	}
	// Try format 5: Raw JSON object (as a last resort)
	if calls, ok := parseRawJSONFormat(content); ok {
		return "", calls, true
	}

	return content, nil, false
}

// parseRawJSONFormat handles a single raw JSON object that might be tool arguments.
func parseRawJSONFormat(content string) ([]ToolCall, bool) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "{") || !strings.HasSuffix(content, "}") {
		return nil, false
	}

	// We don't know the name yet, so we'll have to rely on the model 
	// having been instructed to return the name inside the JSON or 
	// this being a very specific edge case.
	// Actually, some models return {"name": "...", "arguments": {...}}
	var entry struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal([]byte(content), &entry); err == nil && entry.Name != "" {
		return []ToolCall{{
			ID:   "raw-0",
			Type: "function",
			Function: FunctionCall{
				Name:      entry.Name,
				Arguments: string(entry.Arguments),
			},
		}}, true
	}

	return nil, false
}

// parseMarkdownJSONFormat handles tool calls embedded in markdown code blocks.
func parseMarkdownJSONFormat(content string) ([]ToolCall, bool) {
	match := reMarkdownJSON.FindStringSubmatch(content)
	if len(match) == 0 {
		return nil, false
	}

	var rawCalls []struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}

	if err := json.Unmarshal([]byte(match[1]), &rawCalls); err != nil {
		return nil, false
	}

	calls := make([]ToolCall, 0, len(rawCalls))
	for i, rc := range rawCalls {
		calls = append(calls, ToolCall{
			ID:   fmt.Sprintf("md-%d", i),
			Type: "function",
			Function: FunctionCall{
				Name:      rc.Name,
				Arguments: string(rc.Arguments),
			},
		})
	}
	return calls, true
}

// parseNativeFuncFormat handles the functions.name:id{args} style.
func parseNativeFuncFormat(content string) ([]ToolCall, bool) {
	matches := reNativeFunc.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil, false
	}

	calls := make([]ToolCall, 0, len(matches))
	for _, m := range matches {
		name := m[1]
		id := m[2]
		args := m[3]
		calls = append(calls, ToolCall{
			ID:   id,
			Type: "function",
			Function: FunctionCall{
				Name:      name,
				Arguments: args,
			},
		})
	}
	return calls, true
}

// parseFunctionNameFormat handles the <function-name>/<args-json-object> style.
func parseFunctionNameFormat(content string) ([]ToolCall, bool) {
	nameMatches := reToolName.FindAllStringSubmatch(content, -1)
	if len(nameMatches) == 0 {
		return nil, false
	}

	argsMatches := reToolArgs.FindAllStringSubmatch(content, -1)
	calls := make([]ToolCall, 0, len(nameMatches))
	for i, nm := range nameMatches {
		name := strings.TrimSpace(nm[1])
		args := "{}"
		if i < len(argsMatches) {
			args = strings.TrimSpace(argsMatches[i][1])
		}
		calls = append(calls, ToolCall{
			ID:   fmt.Sprintf("cid-%d", i),
			Type: "function",
			Function: FunctionCall{
				Name:      name,
				Arguments: args,
			},
		})
	}
	return calls, true
}

// parseToolsTagFormat handles the <tools>{...}</tools> style where the body is
// a JSON object with "name" and "arguments" keys.
func parseToolsTagFormat(content string) ([]ToolCall, bool) {
	tagMatches := reToolsTag.FindAllStringSubmatch(content, -1)
	if len(tagMatches) == 0 {
		return nil, false
	}

	type toolEntry struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}

	calls := make([]ToolCall, 0, len(tagMatches))
	for i, tm := range tagMatches {
		body := strings.TrimSpace(tm[1])

		var entry toolEntry
		if err := json.Unmarshal([]byte(body), &entry); err != nil || entry.Name == "" {
			continue
		}

		argsJSON, err := json.Marshal(entry.Arguments)
		if err != nil {
			argsJSON = []byte("{}")
		}

		calls = append(calls, ToolCall{
			ID:   fmt.Sprintf("cid-%d", i),
			Type: "function",
			Function: FunctionCall{
				Name:      entry.Name,
				Arguments: string(argsJSON),
			},
		})
	}

	if len(calls) == 0 {
		return nil, false
	}
	return calls, true
}
