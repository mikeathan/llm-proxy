package proxy

import (
	"encoding/json"
	"fmt"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/platform/logging"
	"regexp"
	"strings"
)

// ParseError message constants — defined as named strings for consistency
// across parsers and the agent loop.  Always reference these instead of
// inline literals when assigning to ParseError.JSONError.
const (
	ErrMsgNoValidXMLBlock       = "no valid tool call JSON found in any block"
	ErrMsgNativeMissingFuncName = "native format tool call is missing a function name"
	ErrMsgNoValidNativeBlock    = "no valid tool call found in native format blocks"
	ErrMsgJSONMissingTool       = `missing required field "tool"`
	ErrMsgIncompleteToolCall    = "incomplete tool call — content has tool-call markers but no parseable call"
)

type ParseError struct {
	XMLFound      bool   // true if XML tags were present
	JSONAttempted string // the raw string we tried to parse as JSON (may be truncated)
	JSONError     string // error from json.Unmarshal, if any
	ToolName      string // tool name extracted, if any (may be invalid)
}

func (e *ParseError) Error() string {
	if !e.XMLFound {
		return "no XML tags found in response"
	}
	if e.JSONError != "" {
		return fmt.Sprintf("found XML tags but JSON parse failed: %s", e.JSONError)
	}
	if e.ToolName != "" {
		return fmt.Sprintf("tool %q not recognised", e.ToolName)
	}
	return "unknown parse error"
}

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

// xmlTagPattern matches a <tool_call> block. The closing tag REQUIRES the
// slash (</tool_call>, or the truncated </tool_call without the trailing '>')
// so an opening <tool_call> can never close a block — models that emit a
// doubled opening tag (<tool_call><tool_call>{...}) otherwise produce an empty
// body (the first tag "closes" at the second) and the valid JSON inside is
// lost. The inner JSON is recovered by sanitizeJSON's stripLeadingToolTags.
var xmlTagPattern = regexp.MustCompile(`(?is)<tool(?:_call)?>\s*(.*?)\s*</tool(?:_call)?>?`)

// nativeFormatPattern matches the text tool-call dialects local models emit:
// <function=name>, <tool=name>, and the attributes forms <function name="…">,
// <tool name="…">, <invoke name="…"> (observed from Ornith-1.5 on 2026-08-31).
var nativeFormatPattern = regexp.MustCompile(`(?is)<(function|tool|invoke)(?:\s+name\s*=\s*"([^"]+)"|\s*=\s*([^>\s]+))\s*/?>\s*(.*?)</(?:function|tool|invoke)>`)

var nativeParamTagRe = regexp.MustCompile(`(?is)<parameter\s*>\s*(.*?)\s*</parameter\s*>`)

var nativeParamNamedRe = regexp.MustCompile(`(?is)<parameter\s+name\s*=\s*"([^"]*)"\s*>\s*(.*?)\s*</parameter\s*>`)

var nativeParamEqRe = regexp.MustCompile(`(?is)<parameter\s*=\s*([^>]+)\s*>\s*(.*?)\s*</parameter\s*>`)

// keylessFirstRe matches the malformed "keyless first element" dialect some
// local models emit: an object whose first member is a bare string (the tool
// name) followed by a comma, e.g. {"list_directory", "path": "."} or
// {"list_directory", {"path": "."}}. Such text is never valid JSON (a member
// key is missing), so the match is unambiguous; the repair is gated on strict
// decoding failing first and the tool name is still validated against the
// available tools downstream (ValidateToolCall).
var keylessFirstRe = regexp.MustCompile(`^\{\s*"((?:[^"\\]|\\.)*)"\s*,\s*([\s\S]*)\}\s*$`)

// parseToolCallBlocks is the shared skeleton for parsing repeated tool-call
// blocks: find every match of pattern, parse each via parse, keep the FIRST
// error, and strip matched blocks from the returned content. Matches without a
// parse error produce calls; when a match fails, XMLFound is forced true
// (matches existed) and the error is retained only if no earlier one was kept.
func parseToolCallBlocks(content string, pattern *regexp.Regexp, parse func(match []string, index int) (ToolCall, *ParseError)) (cleaned string, calls []ToolCall, parseErr *ParseError) {
	matches := pattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return content, nil, &ParseError{XMLFound: false}
	}
	cleaned = content
	for i, m := range matches {
		call, err := parse(m, i)
		if err != nil {
			if parseErr == nil {
				parseErr = err
				parseErr.XMLFound = true
			}
			continue
		}
		calls = append(calls, call)
		cleaned = strings.Replace(cleaned, m[0], "", 1)
	}
	return strings.TrimSpace(cleaned), calls, parseErr
}

func ParseContentToolCalls(content string) (cleanedContent string, calls []ToolCall, parseErr *ParseError) {
	cleaned, calls, parseErr := parseToolCallBlocks(content, xmlTagPattern, func(m []string, i int) (ToolCall, *ParseError) {
		return parseSingleToolCall(strings.TrimSpace(m[1]), i)
	})
	if len(calls) == 0 && parseErr == nil {
		parseErr = &ParseError{XMLFound: true, JSONError: ErrMsgNoValidXMLBlock}
	}
	if len(calls) > 0 {
		parseErr = nil
	}
	return cleaned, calls, parseErr
}

func ParseNativeToolCalls(content string) (cleanedContent string, calls []ToolCall, parseErr *ParseError) {
	cleaned, calls, parseErr := parseToolCallBlocks(content, nativeFormatPattern, func(m []string, i int) (ToolCall, *ParseError) {
		toolName := m[2]
		if toolName == "" {
			toolName = m[3]
		}
		if toolName == "" {
			return ToolCall{}, &ParseError{XMLFound: true, JSONError: ErrMsgNativeMissingFuncName}
		}
		args := extractNativeParams(m[4])
		if args == nil {
			args = map[string]string{}
		}
		argsJSON, _ := json.Marshal(args)
		return ToolCall{
			ID:       fmt.Sprintf("tc-%d", i),
			Type:     "function",
			Function: FunctionCall{Name: toolName, Arguments: string(argsJSON)},
		}, nil
	})
	if len(calls) == 0 {
		if parseErr == nil {
			parseErr = &ParseError{XMLFound: true, JSONError: ErrMsgNoValidNativeBlock}
		}
		return cleaned, nil, parseErr
	}
	return cleaned, calls, nil
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
	if len(args) > 0 {
		return args
	}
	return nil
}

// hasTagPrefix reports whether s starts with tag, case-insensitively and
// without allocating (EqualFold, not ToLower).
func hasTagPrefix(s, tag string) bool {
	return len(s) >= len(tag) && strings.EqualFold(s[:len(tag)], tag)
}

// stripLeadingToolTags removes repeated leading <tool_call>/<tool_result>
// opening tags some models emit (<tool_call><tool_call>{...}) — the XML
// extraction starts at the first tag, so the body would otherwise begin with a
// stray tag and fail to parse even though the inner JSON is valid. Runs at most
// once per tool-call attempt and returns immediately for normal input.
func stripLeadingToolTags(s string) string {
	for {
		switch {
		case hasTagPrefix(s, "<tool_call>"):
			s = strings.TrimSpace(s[len("<tool_call>"):])
		case hasTagPrefix(s, "<tool_result>"):
			s = strings.TrimSpace(s[len("<tool_result>"):])
		default:
			return s
		}
	}
}

func sanitizeJSON(raw string) string {
	s := strings.TrimSpace(raw)
	s = stripLeadingToolTags(s)
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
			if esc {
				esc = false
				continue
			}
			if ch == '\\' && inStr {
				esc = true
				continue
			}
			if ch == '"' {
				inStr = !inStr
				continue
			}
			if inStr {
				continue
			}
			if ch == '{' {
				depth++
			}
			if ch == '}' {
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

func replacePythonBooleans(s string) string {
	s = strings.ReplaceAll(s, ": True", ": true")
	s = strings.ReplaceAll(s, ": False", ": false")
	s = strings.ReplaceAll(s, ": None", ": null")
	s = strings.ReplaceAll(s, ":True", ":true")
	s = strings.ReplaceAll(s, ":False", ":false")
	s = strings.ReplaceAll(s, ":None", ":null")
	return s
}

// decodeToolCall decodes a (sanitized) <tool_call> body into a ToolCall.
// Shared by the strict path and the keyless-dialect salvage in
// parseSingleToolCall.
func decodeToolCall(s string, index int) (ToolCall, *ParseError) {
	var call struct {
		Tool string          `json:"tool"`
		Args json.RawMessage `json:"args"`
	}
	if err := json.Unmarshal([]byte(s), &call); err != nil {
		return ToolCall{}, &ParseError{JSONAttempted: truncateForDiagnostic(s), JSONError: err.Error()}
	}
	if call.Tool == "" {
		return ToolCall{}, &ParseError{JSONAttempted: truncateForDiagnostic(s), JSONError: ErrMsgJSONMissingTool}
	}
	args := string(call.Args)
	if args == "" || args == "null" {
		args = "{}"
	}
	return ToolCall{ID: fmt.Sprintf("tc-%d", index), Type: "function", Function: FunctionCall{Name: call.Tool, Arguments: args}}, nil
}

// salvageKeylessToolCall repairs the keyless-first-element dialect: an object
// whose first member is a bare string, e.g. {"list_directory", "path": "."}
// (or {"list_directory", {"path": "."}}). The bare string is the tool name;
// the remainder is the args, wrapped in braces when it is not already an
// object. Returns false unless the remainder is itself valid JSON, so the
// salvage never fabricates a call from garbage.
func salvageKeylessToolCall(s string, index int) (ToolCall, bool) {
	m := keylessFirstRe.FindStringSubmatch(s)
	if m == nil {
		return ToolCall{}, false
	}
	rest := strings.TrimSpace(m[2])
	if rest == "" {
		return ToolCall{}, false
	}
	if !strings.HasPrefix(rest, "{") {
		rest = "{" + rest + "}"
	}
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(rest), &raw); err != nil {
		return ToolCall{}, false
	}
	logging.Warn("repaired keyless tool-call dialect",
		"tool", m[1], "raw", truncateForDiagnostic(s))
	return ToolCall{
		ID:       fmt.Sprintf("tc-%d", index),
		Type:     "function",
		Function: FunctionCall{Name: m[1], Arguments: string(raw)},
	}, true
}

func parseSingleToolCall(jsonStr string, index int) (ToolCall, *ParseError) {
	sanitized := sanitizeJSON(jsonStr)
	call, err := decodeToolCall(sanitized, index)
	if err == nil {
		return call, nil
	}
	// Salvage: only after strict decoding failed, and only for the
	// unambiguous keyless-first-element shape. The tool name is still
	// validated against the available tools downstream (ValidateToolCall).
	if salvaged, ok := salvageKeylessToolCall(sanitized, index); ok {
		return salvaged, nil
	}
	return ToolCall{}, err
}

func ValidateToolCall(call ToolCall, available []Tool) error {
	for _, t := range available {
		if t.Function.Name == call.Function.Name {
			return nil
		}
	}
	return &ParseError{ToolName: call.Function.Name}
}

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
