package proxy

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// xmlToolRegex finds JSON blocks specifically wrapped in <tool_call> tags.
var xmlToolRegex = regexp.MustCompile(`(?is)<tool_call>\s*(.*?)\s*</tool_call>`)

// ParseContentToolCalls extracts tool calls using unambiguous XML boundaries.
func ParseContentToolCalls(content string) (string, []ToolCall, bool) {
	var allCalls []ToolCall
	cleanedContent := content

	matches := xmlToolRegex.FindAllStringSubmatch(content, -1)

	if len(matches) == 0 {
		return content, nil, false // No tool calls found
	}

	for i, match := range matches {
		fullMatch := match[0] // The entire block including XML tags
		jsonStr := match[1]   // The captured JSON inside

		var call struct {
			Tool string          `json:"tool"`
			Args json.RawMessage `json:"args"`
		}

		// Standard Go JSON parsing (Strict)
		if err := json.Unmarshal([]byte(jsonStr), &call); err != nil {
			// Provide an explicit copy-paste error correction to the LLM
			// Ensure the error message itself is JSON-safe
			errMsg := fmt.Sprintf("SYSTEM ERROR: Invalid JSON format inside <tool_call>. Error: %v. You must provide valid JSON.", err)
			errJson, _ := json.Marshal(map[string]string{"error": errMsg})

			errorCall := ToolCall{
				ID:   fmt.Sprintf("parse-error-%d", i),
				Type: "function",
				Function: FunctionCall{
					Name:      "system_error",
					Arguments: string(errJson),
				},
			}
			return content, []ToolCall{errorCall}, true
		}

		if call.Tool != "" {
			allCalls = append(allCalls, ToolCall{
				ID:   fmt.Sprintf("tc-%d", len(allCalls)),
				Type: "function",
				Function: FunctionCall{
					Name:      call.Tool,
					Arguments: string(call.Args),
				},
			})
			// Remove the executed tool block from the visible chat history
			cleanedContent = strings.Replace(cleanedContent, fullMatch, "", 1)
		}
	}

	return strings.TrimSpace(cleanedContent), allCalls, true
}
