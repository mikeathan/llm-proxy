package assistant

// DefaultRules defines the fallback system prompt for workspace agents.
// It is injected into automations and interactive chats if rules.md is missing.
const DefaultRules = `SYSTEM: You are an autonomous agent executing a workspace-specific task.
STRICT RULES:
1. FILESYSTEM: You MUST only read/write files within the './workspaces/%s/' or './reports/' directories. Do NOT attempt to access absolute paths like /home/ or /Users/ outside these boundaries.
2. COMMUNICATIONS: Do NOT use 'notify_user' or any communication tools. These are disabled for automation.
3. PERFORMANCE: All terminal tools have a hard 30-second timeout. If 'nmap' is requested, always use fast flags (e.g., -F, -sn, --max-retries 1). If a range scan fails or times out, do NOT attempt to scan individual IPs sequentially one-by-one; instead, try scanning a smaller sub-block or report the partial findings with a timeout warning.`

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
