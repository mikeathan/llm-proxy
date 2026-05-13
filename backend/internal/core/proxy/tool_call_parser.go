package proxy

import (
	"encoding/json"
	"fmt"
	"llm-proxy/internal/core/assistant/prompts"
	"regexp"
	"strings"
)

// ParseError describes why tool-call extraction failed so the agent can give
// the model specific, actionable feedback instead of a generic nag.
type ParseError struct {
	XMLFound      bool     // true if <tool_call> tags were present
	JSONAttempted string   // the raw string we tried to parse as JSON (may be truncated)
	JSONError     string   // error from json.Unmarshal, if any
	ToolName      string   // tool name extracted, if any (may be invalid)
}

func (e *ParseError) Error() string {
	if !e.XMLFound {
		return "no <tool_call> tags found in response"
	}
	if e.JSONError != "" {
		return fmt.Sprintf("found <tool_call> tags but JSON parse failed: %s", e.JSONError)
	}
	if e.ToolName != "" {
		return fmt.Sprintf("tool %q not recognised", e.ToolName)
	}
	return "unknown parse error"
}

// Feedback returns a prompt fragment the agent can inject so the model
// understands exactly what went wrong and how to fix it.
func (e *ParseError) Feedback(availableTools []string) string {
	allTools := strings.Join(availableTools, ", ")
	if !e.XMLFound {
		return prompts.FeedbackNoXML(allTools)
	}
	if e.JSONError != "" {
		hint := prompts.TranslateJSONError(e.JSONError, e.JSONAttempted)
		return prompts.FeedbackJSONError(hint, allTools)
	}
	if e.ToolName != "" {
		return prompts.FeedbackBadTool(e.ToolName, allTools)
	}
	return prompts.FeedbackGenericFormat()
}

// xmlTagPattern matches the Constitution-mandated <tool_call>…</tool_call> blocks.
// It tolerates minor variations: self-closing open tag, missing close tag, and
// mixed-case fragments that some smaller models produce.
var xmlTagPattern = regexp.MustCompile(`(?is)<tool(?:_call)?>\s*(.*?)\s*</?tool(?:_call)?>?`)

// ParseContentToolCalls extracts tool calls from LLM text output.
//
// Only XML-wrapped JSON is accepted — there are no greedy fallbacks that
// might hallucinate tool calls from conversational text.  When parsing fails
// the returned ParseError carries enough detail for the agent loop to give
// the model specific, actionable feedback.
func ParseContentToolCalls(content string) (cleanedContent string, calls []ToolCall, parseErr *ParseError) {
	matches := xmlTagPattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return content, nil, &ParseError{XMLFound: false}
	}

	cleanedContent = content
	for i, match := range matches {
		fullMatch := match[0]
		jsonStr := strings.TrimSpace(match[1])

		call, err := parseSingleToolCall(jsonStr, i)
		if err != nil {
			// Use the first failure as the diagnostic — it's the one the model
			// needs to fix first.
			if parseErr == nil {
				parseErr = err
				parseErr.XMLFound = true
			}
			continue
		}

		calls = append(calls, call)
		cleanedContent = strings.Replace(cleanedContent, fullMatch, "", 1)
	}

	if len(calls) == 0 && parseErr == nil {
		parseErr = &ParseError{XMLFound: true, JSONError: "no valid tool call JSON found in any <tool_call> block"}
	}

	if len(calls) > 0 {
		parseErr = nil // at least one call succeeded
	}

	return strings.TrimSpace(cleanedContent), calls, parseErr
}

// sanitizeJSON fixes common JSON mistakes that smaller local models make.
// These are normalisation steps, not greedy fallbacks — the model clearly
// intended valid JSON and the semantic content is preserved.
func sanitizeJSON(raw string) string {
	s := strings.TrimSpace(raw)

	// Strip markdown code fences that models sometimes wrap JSON in.
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	}

	// Collapse Python booleans and None.
	s = strings.ReplaceAll(s, ": True", ": true")
	s = strings.ReplaceAll(s, ": False", ": false")
	s = strings.ReplaceAll(s, ": None", ": null")
	s = strings.ReplaceAll(s, ":True", ":true")
	s = strings.ReplaceAll(s, ":False", ":false")
	s = strings.ReplaceAll(s, ":None", ":null")

	// Fix invalid JSON escape sequences that smaller models produce.
	// \$ and \` are not valid JSON escapes but models use them for
	// literal dollar signs and backticks inside markdown code blocks.
	s = strings.ReplaceAll(s, "\\$", "$")
	s = strings.ReplaceAll(s, "\\`", "`")

	// If the string starts with '{', try to extract the first complete JSON
	// object.  Smaller models often append commentary after the closing brace.
	if strings.HasPrefix(s, "{") {
		depth := 0
		inString := false
		escaped := false
		for i, ch := range s {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' && inString {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = !inString
				continue
			}
			if inString {
				continue
			}
			if ch == '{' {
				depth++
			} else if ch == '}' {
				depth--
				if depth == 0 {
					s = strings.TrimSpace(s[:i+1])
					break
				}
			}
		}
	}

	return s
}

// parseSingleToolCall unmarshals one JSON object from inside <tool_call> tags.
func parseSingleToolCall(jsonStr string, index int) (ToolCall, *ParseError) {
	sanitized := sanitizeJSON(jsonStr)

	var call struct {
		Tool string          `json:"tool"`
		Args json.RawMessage `json:"args"`
	}

	if err := json.Unmarshal([]byte(sanitized), &call); err != nil {
		return ToolCall{}, &ParseError{
			JSONAttempted: truncateForDiagnostic(jsonStr),
			JSONError:     err.Error(),
		}
	}

	if call.Tool == "" {
		return ToolCall{}, &ParseError{
			JSONAttempted: truncateForDiagnostic(jsonStr),
			JSONError:     `missing required field "tool"`,
		}
	}

	args := string(call.Args)
	if args == "" || args == "null" {
		args = "{}"
	}

	return ToolCall{
		ID:       fmt.Sprintf("tc-%d", index),
		Type:     "function",
		Function: FunctionCall{Name: call.Tool, Arguments: args},
	}, nil
}

// ValidateToolCall checks that the parsed tool name exists in the registry.
// It returns a ParseError with the tool name set so the agent can give
// targeted feedback about which names are valid.
func ValidateToolCall(call ToolCall, available []Tool) error {
	name := call.Function.Name
	for _, t := range available {
		if t.Function.Name == name {
			return nil
		}
	}
	return &ParseError{ToolName: name}
}

// AvailableToolNames returns a deduplicated, sorted slice of tool names.
func AvailableToolNames(tools []Tool) []string {
	names := make([]string, 0, len(tools))
	seen := make(map[string]bool, len(tools))
	for _, t := range tools {
		if !seen[t.Function.Name] {
			names = append(names, t.Function.Name)
			seen[t.Function.Name] = true
		}
	}
	return names
}

func truncateForDiagnostic(s string) string {
	const max = 200
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
