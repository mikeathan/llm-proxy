# Tool-Failure Investigation

When a tool call fails repeatedly and the model enters a loop or death spiral:

## 1. Read the error, not the model's next action

The server rejected the tool call for a specific reason. The model may be retrying
the same failing call, but that's a symptom of an incomplete recovery path, not
the root cause. Always start by reading the error message at the point of failure.

## 2. Find all error handlers for the failing tool

Search the codebase for every error-handling path the tool's failure touches.
A tool typically has error handling in multiple layers:

- **The handler itself** (e.g. `filesystem.go` returns the error)
- **The executor** (e.g. `tool_exec.go` catches validation failures)
- **The session loop** (e.g. `session.go` routes parse errors to prompts)

Each layer may have tool-specific logic. All of them need checking.

## 3. Check handler scope

If an error handler checks `toolName == "write_file"` but the failing tool is
something else (e.g. `edit_file_block`), the scope is wrong. The model gets
a generic error instead of actionable guidance. This is the most common bug.

## 4. Check the recovery prompt

Is the prompt specific to the actual problem? If the error is "JSON parse
failure from large arguments" but the prompt says "use write_file + append_file,"
the prompt may be misleading for the failing tool. The model follows the prompt
and keeps failing because the advice doesn't match the tool.

## 5. Check if the model should be using a different tool

If `edit_file_block` keeps failing with large replacement blocks, maybe
`write_file` (which has no content limit) is the right tool for that
situation. The error handler should guide the model to switch tools, not
retry the same call with smaller content.

## 6. Verify with code, not assumption

Before attributing a failure to model capability, inspect every code path
the error touches. The pattern is: error → handler → prompt → model retry.
If any link in this chain has a gap, that's the fix, not the model.
