# SPEC: Tool Call Parser

## I. Intent
The tool call parser (`proxy/tool_call_parser.go`) extracts structured tool calls from LLM text output. It accepts ONLY XML-wrapped JSON — there are no greedy fallbacks that might hallucinate tool calls from conversational text. When parsing fails, it returns a structured `ParseError` that the agent loop can use to give the model specific, actionable feedback.

## II. Functional Requirements

### 1. Accepted Format
Only this pattern is recognized:
```
<tool_call>
{"tool": "tool_name", "args": {"arg1": "value1"}}
</tool_call>
```

The regex tolerates minor variations that smaller models produce:
- Self-closing open tag: `<tool_call/>` or `<tool_call>`
- Missing slash in close tag: `</tool_call` or `</tool_call>` or `</tool>`
- Mixed-case fragments

### 2. Rejected Formats (by design)
- Naked JSON without XML wrapper: `{"tool": "read_file", "args": {...}}`
- Markdown code fences: ` ```json ... ``` `
- Old-style function-name tags: `<function-name>...</function-name>`
- Pipe-style tool calls: `<|tool_call>...</|tool_call>`
- Key-value detection: content containing `"tool":` and `"args":` anywhere

These were removed because they caused false positives from conversational text.

### 3. ParseError Structure
```go
type ParseError struct {
    XMLFound      bool   // true if <tool_call> tags were present
    JSONAttempted string // raw string we tried to parse (truncated to 200 chars)
    JSONError     string // error from json.Unmarshal, if any
    ToolName      string // tool name extracted, if any (may be invalid)
}
```

### 4. ParseError.Feedback()
Generates a prompt fragment tailored to the specific failure:

- **No XML tags**: "FORMAT ERROR: No <tool_call> tags found. You MUST wrap your tool call like this: `<tool_call>{"tool":"TOOL_NAME","args":{...}}</tool_call>`. Available tools: ..."
- **JSON parse failed**: "FORMAT ERROR: Found <tool_call> tags but the JSON inside was invalid: [error]. Ensure you use double-quotes for keys and string values, no trailing commas. Available tools: ..."
- **Unknown tool**: "TOOL ERROR: Unknown tool '[name]'. Available tools: ..."

A separate prompt (`AutomationContentTooLongPrompt` in `templates.go`) handles the case where the model's `write_file` content exceeds the output limit — it instructs the model to use `write_file` + `append_file` for multi-chunk writes.

### 5. Tool Validation
`ValidateToolCall(call, availableTools)` checks the parsed tool name against the registry. Returns a `ParseError` with `ToolName` set if the tool is not recognized.

### 6. Helper Functions
- `AvailableToolNames(tools)`: returns deduplicated, sorted tool names for feedback messages.
- `truncateForDiagnostic(s)`: caps diagnostic strings at 200 characters.

## III. Why No Greedy Fallbacks

The original parser had 3 phases:
1. XML tags (kept)
2. Naked JSON: grabbed from the first `{` to the last `}` in the entire response — created phantom tool calls from text like "I checked the {config file}"
3. Ultra-greedy key-value: wrapped content in `{...}` if it contained `"tool":` and `"args":` anywhere

Phases 2 and 3 caused more false positives than true recoveries. If a model can't produce valid `<tool_call>` XML, the correct response is specific format feedback, not desperate guesswork.

## IV. Testing Strategy
- Table-driven tests in `tool_call_parser_test.go`.
- Coverage: standard XML format, multiple tool calls, malformed JSON, missing tool field, old-style tags (rejected), raw JSON (rejected), fuzzy close tags, self-closing tags, `ValidateToolCall`, `ParseError.Feedback` for each error type, `AvailableToolNames` deduplication.
