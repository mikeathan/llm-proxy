package proxy

import (
	"encoding/json"
	"fmt"
	"llm-proxy/internal/core/assistant/prompts"
	"regexp"
	"strings"
)

type ParseError struct {
	XMLFound      bool     // true if XML tags were present
	JSONAttempted string   // the raw string we tried to parse as JSON (may be truncated)
	JSONError     string   // error from json.Unmarshal, if any
	ToolName      string   // tool name extracted, if any (may be invalid)
}

func (e *ParseError) Error() string {
	if !e.XMLFound { return "no XML tags found in response" }
	if e.JSONError != "" { return fmt.Sprintf("found XML tags but JSON parse failed: %s", e.JSONError) }
	if e.ToolName != "" { return fmt.Sprintf("tool %q not recognised", e.ToolName) }
	return "unknown parse error"
}

func (e *ParseError) Feedback(availableTools []string) string {
	allTools := strings.Join(availableTools, ", ")
	if !e.XMLFound { return prompts.FeedbackNoXML(allTools) }
	if e.JSONError != "" { hint := prompts.TranslateJSONError(e.JSONError, e.JSONAttempted); return prompts.FeedbackJSONError(hint, allTools) }
	if e.ToolName != "" { return prompts.FeedbackBadTool(e.ToolName, allTools) }
	return prompts.FeedbackGenericFormat()
}

var xmlTagPattern = regexp.MustCompile(`(?is)<tool(?:_call)?>\s*(.*?)\s*</?tool(?:_call)?>?`)

var nativeFormatPattern = regexp.MustCompile(`(?is)<(function|tool)(?:\s+name\s*=\s*"([^"]+)"|\s*=\s*([^>\s]+))\s*/?>\s*(.*?)</(?:function|tool)>`)

var nativeParamTagRe = regexp.MustCompile(`(?is)<parameter\s*>\s*(.*?)\s*</parameter\s*>`)

var nativeParamNamedRe = regexp.MustCompile(`(?is)<parameter\s+name\s*=\s*"([^"]*)"\s*>\s*(.*?)\s*</parameter\s*>`)

var nativeParamEqRe = regexp.MustCompile(`(?is)<parameter\s*=\s*([^>]+)\s*>\s*(.*?)\s*</parameter\s*>`)

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
		parseErr = &ParseError{XMLFound: true, JSONError: "no valid tool call JSON found in any block"}
	}
	if len(calls) > 0 { parseErr = nil }
	return strings.TrimSpace(cleanedContent), calls, parseErr
}

func ParseNativeToolCalls(content string) (cleanedContent string, calls []ToolCall, parseErr *ParseError) {
	matches := nativeFormatPattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return content, nil, &ParseError{XMLFound: false}
	}
	cleaned := content
	foundAny := false
	for i, m := range matches {
		fullMatch := m[0]
		toolName := ""
		if len(m) > 2 { toolName = m[2] }
		if toolName == "" && len(m) > 3 { toolName = m[3] }
		if toolName == "" { continue }
		inner := m[4]
		args := extractNativeParams(inner)
		if args == nil { args = map[string]string{} }
		argsJSON, _ := json.Marshal(args)
		calls = append(calls, ToolCall{
			ID: fmt.Sprintf("tc-%d", i),
			Type: "function",
			Function: FunctionCall{Name: toolName, Arguments: string(argsJSON)},
		})
		foundAny = true
		cleaned = strings.Replace(cleaned, fullMatch, "", 1)
	}
	if !foundAny {
		return content, nil, &ParseError{XMLFound: false}
	}
	return strings.TrimSpace(cleaned), calls, nil
}

func extractNativeParams(inner string) map[string]string {
	args := make(map[string]string)
	named := nativeParamNamedRe.FindAllStringSubmatch(inner, -1)
	for _, p := range named {
		args[p[1]] = strings.TrimSpace(p[2])
	}
	eq := nativeParamEqRe.FindAllStringSubmatch(inner, -1)
	for _, p := range eq {
		args[p[1]] = strings.TrimSpace(p[2])
	}
	simple := nativeParamTagRe.FindAllStringSubmatch(inner, -1)
	for _, p := range simple {
		args[fmt.Sprintf("arg%d", len(args))] = strings.TrimSpace(p[1])
	}
	if len(args) > 0 { return args }
	return nil
}

func sanitizeJSON(raw string) string {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	}
	s = replacePythonBooleans(s)
	s = strings.ReplaceAll(s, "\\$", "$")
	s = strings.ReplaceAll(s, "\\`", "`")
	if strings.HasPrefix(s, "{") {
		depth := 0
		inStr := false
		esc := false
		for i, ch := range s {
			if esc { esc = false; continue }
			if ch == '\\' && inStr { esc = true; continue }
			if ch == '"' { inStr = !inStr; continue }
			if inStr { continue }
			if ch == '{' { depth++ }
			if ch == '}' { depth--; if depth == 0 { s = strings.TrimSpace(s[:i+1]); break } }
		}
	}
	return s
}

func replacePythonBooleans(s string) string {
	s = strings.ReplaceAll(s, ": True", ": true")
	s = strings.ReplaceAll(s, ": False", ": false")
	s = strings.ReplaceAll(s, ": None", ": null")
	s = strings.ReplaceAll(s, ":True", ":true")
	s = strings.ReplaceAll(s, ":False", ":false")
	s = strings.ReplaceAll(s, ":None", ":null")
	return s
}

func parseSingleToolCall(jsonStr string, index int) (ToolCall, *ParseError) {
	sanitized := sanitizeJSON(jsonStr)
	var call struct { Tool string `json:"tool"`; Args json.RawMessage `json:"args"` }
	if err := json.Unmarshal([]byte(sanitized), &call); err != nil {
		return ToolCall{}, &ParseError{JSONAttempted: truncateForDiagnostic(jsonStr), JSONError: err.Error()}
	}
	if call.Tool == "" {
		return ToolCall{}, &ParseError{JSONAttempted: truncateForDiagnostic(jsonStr), JSONError: `missing required field "tool"`}
	}
	args := string(call.Args)
	if args == "" || args == "null" { args = "{}" }
	return ToolCall{ID: fmt.Sprintf("tc-%d", index), Type: "function", Function: FunctionCall{Name: call.Tool, Arguments: args}}, nil
}

func ValidateToolCall(call ToolCall, available []Tool) error {
	for _, t := range available { if t.Function.Name == call.Function.Name { return nil } }
	return &ParseError{ToolName: call.Function.Name}
}

func AvailableToolNames(tools []Tool) []string {
	names := make([]string, 0, len(tools))
	seen := make(map[string]bool, len(tools))
	for _, t := range tools {
		if !seen[t.Function.Name] { names = append(names, t.Function.Name); seen[t.Function.Name] = true }
	}
	return names
}

func truncateForDiagnostic(s string) string {
	const max = 200
	if len(s) <= max { return s }
	return s[:max] + "..."
}
