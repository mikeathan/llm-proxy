package proxy

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var jsonBlockRegex = regexp.MustCompile("(?is)```json\\s*(.*?)\\s*```")
var plainBlockRegex = regexp.MustCompile("(?is)```\\s*({.*?})\\s*```")

// ParseContentToolCalls extracts a tool call from Markdown JSON blocks.
// Open Claw v2: Pure text-in/text-out interface using standard Markdown code blocks.
func ParseContentToolCalls(content string) (string, []ToolCall, bool) {
	match := jsonBlockRegex.FindStringSubmatch(content)
	if len(match) < 2 {
		// Fallback to plain markdown block if it looks like JSON
		match = plainBlockRegex.FindStringSubmatch(content)
		if len(match) < 2 {
			return content, nil, false
		}
	}

	jsonStr := strings.TrimSpace(match[1])
	fullMatch := match[0]
	startIdx := strings.Index(content, fullMatch)

	// 4. Run standard Go json.Unmarshal
	var call struct {
		Tool string          `json:"tool"`
		Args json.RawMessage `json:"args"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &call); err != nil {
		// Open Claw v2 Phase 4: Return a structured error string to the loop
		errorCall := ToolCall{
			ID:   "parse-error",
			Type: "function",
			Function: FunctionCall{
				Name: "system_error",
				Arguments: fmt.Sprintf(`{"error": "SYSTEM ERROR: Malformed action format. You must format your action as a single Markdown JSON block. If you are finished, you MUST reply with exactly this:\n\n`+"```json"+`\n{\n  \"tool\": \"submit_final_answer\",\n  \"args\": {\n    \"summary\": \"your final results here\"\n  }\n}\n`+"```"+`"}`, 
				),
			},
		}
		return content, []ToolCall{errorCall}, true
	}

	if call.Tool == "" {
		return content, nil, false
	}

	tc := ToolCall{
		ID:   "tc-0",
		Type: "function",
		Function: FunctionCall{
			Name:      call.Tool,
			Arguments: string(call.Args),
		},
	}

	// Clean the content by removing the JSON block
	cleaned := content[:startIdx] + content[startIdx+len(fullMatch):]
	return strings.TrimSpace(cleaned), []ToolCall{tc}, true
}
