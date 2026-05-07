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
// : Production-grade robustness handling multiple calls, escaped quotes, and case-insensitivity.
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
				// FORGIVENESS: If no backticks are found, try to find the first raw '{'
				startIdx = strings.Index(workingContent, "{")
				if startIdx == -1 {
					break
				}
				marker = "" // No marker prefix
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
			
			// Apply recovery even to cutoffs
			repairedJson := repairJsonHeuristic(jsonStr)
			
			var call struct {
				Tool string          `json:"tool"`
				Args json.RawMessage `json:"args"`
			}
			if err := json.Unmarshal([]byte(repairedJson), &call); err == nil && call.Tool != "" {
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

		// 6. Parse and Collect with Lax Recovery
		var call struct {
			Tool string          `json:"tool"`
			Args json.RawMessage `json:"args"`
		}

		repairedJson := repairJsonHeuristic(jsonStr)
		if err := json.Unmarshal([]byte(repairedJson), &call); err == nil && call.Tool != "" {
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
			// Ensure the error message itself is JSON-safe
			errMsg := fmt.Sprintf("SYSTEM ERROR: Malformed action format. Error: %v", err)
			errJson, _ := json.Marshal(map[string]string{"error": errMsg})
			
			errorCall := ToolCall{
				ID:   "parse-error",
				Type: "function",
				Function: FunctionCall{
					Name:      "system_error",
					Arguments: string(errJson),
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

// repairJsonHeuristic cleans up common LLM JSON hallucinations and formatting errors.
func repairJsonHeuristic(jsonStr string) string {
	// 1. Escape raw newlines inside what look like string values
	// LLMs often output literal newlines instead of \n
	var result strings.Builder
	inString := false
	for i := 0; i < len(jsonStr); i++ {
		char := jsonStr[i]
		if char == '"' {
			// Basic escape check
			backslashes := 0
			for j := i - 1; j >= 0; j-- {
				if jsonStr[j] == '\\' {
					backslashes++
				} else {
					break
				}
			}
			if backslashes%2 == 0 {
				inString = !inString
			}
			result.WriteByte(char)
		} else if char == '\n' || char == '\r' {
			if inString {
				result.WriteString("\\n")
			} else {
				result.WriteByte(char)
			}
		} else {
			result.WriteByte(char)
		}
	}
	jsonStr = result.String()

	// 2. Strip trailing commas before closing braces/brackets
	re := regexp.MustCompile(`,(\s*[}\]])`)
	jsonStr = re.ReplaceAllString(jsonStr, "$1")

	// 3. Map common hallucinated keys ("name" -> "tool", "arguments" -> "args")
	jsonStr = strings.ReplaceAll(jsonStr, `"name":`, `"tool":`)
	jsonStr = strings.ReplaceAll(jsonStr, `"arguments":`, `"args":`)

	// 4. Handle unquoted keys (e.g., {tool: "x", args: {}})
	// Improved to only match keys at start of line or after { ,
	keyRe := regexp.MustCompile(`(?m)([{,]\s*)([a-zA-Z0-9_]+):`)
	jsonStr = keyRe.ReplaceAllString(jsonStr, `$1"$2":`)

	// 5. Fix common internal quote failures (unescaped quotes in values)
	// Heuristic: If we see " followed by a word and then another ", but no colon/comma/brace follows, 
	// it might be an unescaped internal quote. This is aggressive but helpful for LLM output.
	// We'll just do a common pattern: replace " with \" if it's preceded by a space and followed by a word
	// (Very conservative to avoid breaking the JSON structure)
	// Actually, let's just use the double-wrap unwrapper first.

	// 6. Handle double-wrapped JSON (e.g., {"action": {"tool": "x", ...}})
	// ONLY if we don't already have a "tool" key at the top level
	if !strings.Contains(jsonStr, `"tool":`) {
		start := strings.Index(jsonStr, "{")
		if start != -1 {
			innerStart := strings.Index(jsonStr[start+1:], "{")
			if innerStart != -1 {
				innerStart += start + 1
				lastBrace := strings.LastIndex(jsonStr, "}")
				if lastBrace != -1 && lastBrace > innerStart {
					innerJson := jsonStr[innerStart : lastBrace]
					if strings.Contains(innerJson, `"tool":`) || strings.Contains(innerJson, `"name":`) {
						return jsonStr[innerStart : lastBrace+1]
					}
				}
			}
		}
	}

	return jsonStr
}
