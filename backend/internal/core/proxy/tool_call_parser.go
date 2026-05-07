package proxy

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var jsonBlockRegex = regexp.MustCompile("(?is)```json\\s*(.*?)\\s*```")
var plainBlockRegex = regexp.MustCompile("(?is)```\\s*({.*?})\\s*```")

// ParseContentToolCalls extracts tool calls from Markdown JSON blocks.
// Open Claw v3: Production-grade robustness handling multiple calls, escaped quotes, and case-insensitivity.
func ParseContentToolCalls(content string) (string, []ToolCall, bool) {
	var allCalls []ToolCall
	var cleanedParts []string
	workingContent := content
	lastPos := 0
	offset := 0

	for {
		// 1. Find the start of the JSON block (Case-Insensitive)
		lowerContent := strings.ToLower(workingContent)
		marker := "```json"
		startIdx := strings.Index(lowerContent, marker)
		if startIdx == -1 {
			marker = "```"
			startIdx = strings.Index(lowerContent, marker)
			if startIdx == -1 {
				break
			}
		}

		// 2. Find the first '{' after the marker
		jsonStart := strings.Index(workingContent[startIdx+len(marker):], "{")
		if jsonStart == -1 {
			// Skip this block and continue searching
			workingContent = workingContent[startIdx+len(marker):]
			offset += startIdx + len(marker)
			continue
		}
		jsonStart += startIdx + len(marker)

		// 3. Find the matching '}' with escape-aware balanced-brace counting
		braceCount := 0
		inString := false
		jsonEnd := -1
		
		for i := jsonStart; i < len(workingContent); i++ {
			char := workingContent[i]
			
			// Escape-aware string detection
			if char == '"' {
				backslashes := 0
				for j := i - 1; j >= 0; j-- { // Scan backwards in workingContent
					if workingContent[j] == '\\' {
						backslashes++
					} else {
						break
					}
				}
				if backslashes%2 == 0 {
					inString = !inString
				}
			}

			if !inString {
				if char == '{' {
					braceCount++
				} else if char == '}' {
					braceCount--
					if braceCount == 0 {
						jsonEnd = i + 1
						break
					}
				}
			}
		}

		// 4. Greedy Fallback: Handle mid-generation cutoffs
		if jsonEnd == -1 && braceCount > 0 {
			jsonStr := strings.TrimSpace(workingContent[jsonStart:])
			if inString {
				jsonStr += "\""
			}
			for braceCount > 0 {
				jsonStr += "}"
				braceCount--
			}
			
			var call struct {
				Tool string          `json:"tool"`
				Args json.RawMessage `json:"args"`
			}
			if err := json.Unmarshal([]byte(jsonStr), &call); err == nil && call.Tool != "" {
				allCalls = append(allCalls, ToolCall{
					ID:   fmt.Sprintf("tc-cutoff-%d", len(allCalls)),
					Type: "function",
					Function: FunctionCall{
						Name:      call.Tool,
						Arguments: string(call.Args),
					},
				})
				// Collect text before the block and stop (it was cut off)
				cleanedParts = append(cleanedParts, content[lastPos:offset+startIdx])
				lastPos = len(content) // Skip everything else
				break
			}
		}

		if jsonEnd == -1 {
			workingContent = workingContent[startIdx+len(marker):]
			offset += startIdx + len(marker)
			continue
		}

		jsonStr := workingContent[jsonStart:jsonEnd]
		
		// 5. Find the closing backticks
		fullMatchEnd := jsonEnd
		if idx := strings.Index(workingContent[jsonEnd:], "```"); idx != -1 {
			fullMatchEnd = jsonEnd + idx + 3
		}

		// 6. Parse and Collect
		var call struct {
			Tool string          `json:"tool"`
			Args json.RawMessage `json:"args"`
		}

		if err := json.Unmarshal([]byte(jsonStr), &call); err == nil && call.Tool != "" {
			allCalls = append(allCalls, ToolCall{
				ID:   fmt.Sprintf("tc-%d", len(allCalls)),
				Type: "function",
				Function: FunctionCall{
					Name:      call.Tool,
					Arguments: string(call.Args),
				},
			})
			
			// Collector Pattern: Collect text BEFORE the block
			cleanedParts = append(cleanedParts, content[lastPos:offset+startIdx])
			lastPos = offset + fullMatchEnd
		} else if err != nil && len(allCalls) == 0 {
			errorCall := ToolCall{
				ID:   "parse-error",
				Type: "function",
				Function: FunctionCall{
					Name: "system_error",
					Arguments: fmt.Sprintf(`{"error": "SYSTEM ERROR: Malformed action format. Error: %v"}`, err),
				},
			}
			return content, []ToolCall{errorCall}, true
		}

		// Move to the next part
		workingContent = workingContent[fullMatchEnd:]
		offset += fullMatchEnd
		if len(workingContent) < 10 {
			break
		}
	}

	if len(allCalls) == 0 {
		return content, nil, false
	}

	// Append remaining text after the last block
	if lastPos < len(content) {
		cleanedParts = append(cleanedParts, content[lastPos:])
	}

	return strings.TrimSpace(strings.Join(cleanedParts, "\n")), allCalls, true
}
