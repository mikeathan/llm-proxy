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
)

// ParseContentToolCalls inspects a message's Content for embedded tool call
// markup. If found, it returns a synthesised ToolCall slice and true. The
// caller should then treat the message as if it had proper ToolCalls and clear
// the Content field to avoid returning it as a final answer.
func ParseContentToolCalls(content string) ([]ToolCall, bool) {
	// Try format 1: <function-name> / <args-json-object>
	if calls, ok := parseFunctionNameFormat(content); ok {
		return calls, true
	}
	// Try format 2: <tools> wrapping a JSON object
	if calls, ok := parseToolsTagFormat(content); ok {
		return calls, true
	}
	return nil, false
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
