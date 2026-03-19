package proxy

import (
	"fmt"
	"regexp"
	"strings"
)

// These patterns match the XML-like tool call format used by some local models
// such as qwen2.5-coder, instead of the standard OpenAI tool_calls JSON field.
//
// Example content the model emits:
//
//	<function-name>query_device</function-name>
//	<args-json-object>
//	{"target_name": "Living Room Light", "metrics": ["state"]}
//	</args-json-object>
var (
	reToolName = regexp.MustCompile(`(?s)<function-name>(.*?)</function-name>`)
	reToolArgs = regexp.MustCompile(`(?s)<args-json-object>(.*?)</args-json-object>`)
)

// ParseContentToolCalls inspects a message's Content for embedded tool call
// markup. If found, it returns a synthesised ToolCall slice and true. The
// caller should then treat the message as if it had proper ToolCalls and clear
// the Content field to avoid returning it as a final answer.
func ParseContentToolCalls(content string) ([]ToolCall, bool) {
	nameMatches := reToolName.FindAllStringSubmatch(content, -1)
	argsMatches := reToolArgs.FindAllStringSubmatch(content, -1)

	if len(nameMatches) == 0 {
		return nil, false
	}

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
