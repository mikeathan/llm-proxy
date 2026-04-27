package prompts

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FileSystemRules defines the standard path jail instructions.
const FileSystemRules = `
STRICT WORKSPACE RULES:
1. FILESYSTEM: All file paths MUST be relative to the workspace root. Example: to read 'task.md', use 'task.md'.`

const ToolManualHeader = "# TOOL INTERFACE"

// HasToolManual checks if the tool instructions are already present in the content.
func HasToolManual(content string) bool {
	return strings.Contains(content, ToolManualHeader)
}

// InjectToolManual merges tool instructions into existing content, ensuring they are only added once.
func InjectToolManual(content string, instructions string) string {
	if HasToolManual(content) {
		return content
	}
	if content == "" {
		return instructions
	}
	return fmt.Sprintf("%s\n\n%s", content, instructions)
}

// ToolInfo is a simplified version of a tool's schema for template generation.
type ToolInfo struct {
	Name        string
	Description string
	Parameters  any
}

// BuildToolManual generates the dynamic technical manual for available tools.
// It uses the unified JSON array format for consistency.
func BuildToolManual(tools []ToolInfo) string {
	if len(tools) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, t := range tools {
		sb.WriteString(fmt.Sprintf("### %s\n%s\n", t.Name, t.Description))
		raw, _ := json.MarshalIndent(t.Parameters, "", "  ")
		sb.WriteString(fmt.Sprintf("Parameters:\n```json\n%s\n```\n\n", string(raw)))
		params, _ := json.Marshal(t.Parameters)
		sb.WriteString(fmt.Sprintf("#### %s\n%s\nArguments Schema: %s\n\n", t.Name, t.Description, string(params)))
	}

	return fmt.Sprintf(UnifiedToolManual, sb.String())
}

// UnifiedToolManual defines the ONE canonical tool calling format.
// This is intentionally strict: one format, concrete examples, no ambiguity.
const UnifiedToolManual = `## TOOL INTERFACE
You have access to technical tools. To use a tool, you MUST output a JSON array inside a markdown code block.

### Format
` + "```" + `json
[
  {
    "tool": "tool_name",
    "args": { "arg_name": "value" }
  }
]
` + "```" + `

### Rules
1. You can call multiple tools in one block. They will execute sequentially.
2. Use ONLY the tools listed below.
3. If a tool fails, you will receive the error. Fix it in your next turn.
4. When finished, call 'submit_task' with your final report.

### Available Tools
%s`

// DefaultRules is the operational protocol injected into the system prompt.
// It does NOT mention tool calling format — that lives in UnifiedToolManual.
const DefaultRules = `You are an autonomous agent with access to tools. Your job is to complete the given task by using tools.

RULES:
1. Use tools to act. Do not describe what you would do — do it.
2. Verify results. After each action, check the output before proceeding.
3. Fix errors immediately. If a tool fails, analyze and retry.
4. Do not assume. Check the workspace state with ls/cat before assuming files exist.
5. Complete the task. When finished and verified, call submit_task with your report.
`

// AssembleSystemPrompt aggregates the core operational constitution with any workspace-specific rules.
func AssembleSystemPrompt(customRules string) string {
	prompt := DefaultRules
	if customRules != "" && strings.TrimSpace(customRules) != strings.TrimSpace(DefaultRules) {
		prompt += "\n\nWORKSPACE-SPECIFIC RULES:\n" + customRules
	}
	return prompt
}

// DefaultHeartbeat defines a generic placeholder automation task.
const DefaultHeartbeat = `# Heartbeat Task
# Add your instructions here.
Example: Scan the local directory and list files.
`

// DefaultAgentPrompt defines the basic persona for the interactive workspace assistant.
const DefaultAgentPrompt = `# Workspace Agent
You are the interactive assistant for this workspace. Use the available tools to help the user.
You have access to the local filesystem and terminal.
Guidelines:
1. Be concise.
2. Prefer using tools to gather information before answering.
3. Stay within the authorized workspace boundaries.`

// LocalAssistantPrompt defines the persona for the LocalToolRegistry.
const LocalAssistantPrompt = DefaultRules

// DefaultWorkspaceConfig defines a clean, empty configuration for new workspaces.
const DefaultWorkspaceConfig = `model: ""
temperature: 0.7
automations: []`

const AutomationMarker = "TASK: You are an autonomous agent"

func IsAutomationTask(content string) bool {
	return strings.Contains(content, AutomationMarker)
}

// AutomationNagPrompt is sent when a model outputs text without any tool calls.
const AutomationNagPrompt = `You must use a tool to continue. Output your actions in a JSON markdown block:

` + "```" + `json
[
  {
    "tool": "execute_terminal_command",
    "args": { "command": "your command here" }
  }
]
` + "```" + `

Or if you are finished:

` + "```" + `json
[
  {
    "tool": "submit_task",
    "args": { "summary": "your markdown report" }
  }
]
` + "```" + `
`

// AutomationJSONPlanPrompt is a fallback for models that cannot emit XML tool calls.
// It asks the model to output its intended actions as a JSON array so the backend
// can execute the plan directly without relying on XML parsing.
const AutomationJSONPlanPrompt = `XML tool calling failed. Switch to JSON PLAN MODE.
Now output your full plan as a JSON array so the system can execute it.

Output ONLY a JSON array. No text before or after. Each element must have "tool" and "args" fields.

Available tools: execute_terminal_command, write_file, read_file, list_directory, submit_task

Example:
[
  {"tool": "execute_terminal_command", "args": {"command": "mkdir -p project/src"}},
  {"tool": "write_file", "args": {"path": "project/src/main.ts", "content": "console.log('hello')"}},
  {"tool": "execute_terminal_command", "args": {"command": "node project/src/main.js"}}
]`

// AutomationTaskPrompt is the user-facing task message for autonomous agents.
// It references the same tool format as UnifiedToolManual.
const AutomationTaskPrompt = AutomationMarker + ` in workspace '%s'.
Execute the instructions found in '%s':
---
%s
---

Use your tools to complete every step. Call submit_task when done.`
