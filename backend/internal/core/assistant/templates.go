package assistant

import (
	"strings"
)

// FileSystemRules defines the standard path jail instructions.
const FileSystemRules = `1. FILESYSTEM: All file paths MUST be relative to the application root. To access files in your current workspace, you MUST prefix the path with '{{REL_WS}}/{{WORKSPACE_ID}}/'. Example: to read 'task.md', use '{{REL_WS}}/{{WORKSPACE_ID}}/task.md'.`

// BuildJailPrompt constructs the strict workspace filesystem jail rules.
func BuildJailPrompt(relWs, workspaceID string) string {
	res := "\n\nSTRICT WORKSPACE RULES:\n" + FileSystemRules
	res = strings.ReplaceAll(res, "{{REL_WS}}", relWs)
	res = strings.ReplaceAll(res, "{{WORKSPACE_ID}}", workspaceID)
	return res
}

// DefaultRules defines the fallback system prompt for workspace agents.
// It is injected into automations and interactive chats if rules.md is missing.
const DefaultRules = `SYSTEM: You are an autonomous agent executing a workspace-specific task.
STRICT RULES:
` + FileSystemRules + `
2. OUTPUT FORMAT: Your response must be plain text or Markdown. NEVER return raw JSON arrays like '[{"type": "text", ...}]'. When providing your final markdown summary, do NOT wrap the entire response in markdown code blocks (e.g. using triple backticks). Provide the raw markdown directly so it can be rendered as a document.
3. COMMUNICATIONS: Do NOT use 'notify_user' or any communication tools. These are disabled for automation.
4. PERFORMANCE: All terminal tools have a hard 60-second timeout. If 'nmap' is requested, always use fast flags (e.g., -F, -sn, --max-retries 1) unless a deeper scan is explicitly required. If a range scan fails or times out, do NOT attempt to scan individual IPs sequentially one-by-one; instead, try scanning a smaller sub-block or report the partial findings with a timeout warning.
5. COMMAND EXECUTION: When executing terminal tools, always rely on reading the standard output directly. Do not use output file flags (e.g. -oN), shell pipes (|), or redirections (>) to write to /tmp or outside the authorized workspace, as these will be aggressively blocked by security guardrails.

TOOL CALL FORMAT:
To use a tool, you MUST use the following XML-like structure in your response:
<function-name>name_of_the_tool</function-name>
<args-json-object>{"arg1": "value1"}</args-json-object>

Example:
<function-name>execute_terminal_command</function-name>
<args-json-object>{"command": "ls -la {{REL_WS}}/{{WORKSPACE_ID}}/"}</args-json-object>`

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

// DefaultWorkspaceConfig defines a clean, empty configuration for new workspaces.
const DefaultWorkspaceConfig = `model: ""
temperature: 0.7
automations: []`

// AutomationTaskPrompt defines the standard instruction wrapper for automation runs.
const AutomationTaskPrompt = `%s

TASK: You are an autonomous agent in workspace '%s'. 
Execute the instructions found in '%s/%s/%s':
---
%s
---

Follow the execution steps exactly. Use your tools to perform the task. 
Once finished, provide a concise markdown summary of your findings. DO NOT return empty responses.`
