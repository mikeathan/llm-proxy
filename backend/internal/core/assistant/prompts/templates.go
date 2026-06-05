package prompts

import (
	"encoding/json"
	"fmt"
	"strings"

	"llm-proxy/models"
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
		params, _ := json.MarshalIndent(t.Parameters, "", "  ")
		sb.WriteString(fmt.Sprintf("Schema:\n```json\n%s\n```\n\n", string(params)))
	}
	return fmt.Sprintf(UnifiedToolManual, sb.String())
}

// UnifiedToolManual defines the ONE canonical tool calling format using XML boundaries.
const UnifiedToolManual = `## TOOL INTERFACE
You operate in a strict Reason -> Act -> Observe loop. 
To execute a tool, you MUST wrap a single valid JSON object inside <tool_call> tags.
Do not use markdown block formatting (` + "```json" + `) inside the tags.

Format:
Thought: [Your reasoning here]
Action:
<tool_call>
{
  "tool": "TOOL_NAME",
  "args": {
    "ARG_NAME": "VALUE"
  }
}
</tool_call>

### Rules
1. **Parallel Tool Execution**: Batch related actions into a single response using multiple <tool_call> tags to improve efficiency.
2. Use ONLY the tools listed below.
3. If a tool fails, you will receive the error. Fix it in your next turn.
4. Finalization: You are not finished until you call 'submit_final_answer'. 
    - **Thought**: Use this for your internal reasoning (e.g., "I have all data, I will now finalize").
    - **Action**: Use 'submit_final_answer'. The 'summary' argument MUST be the actual comprehensive report containing all raw data, tables, and findings.

### Available Tools
%s`

// NativeToolReference lists available tools without XML format instructions.
// Used when native tool calling is active — the LLM receives tool schemas
// via the API but still needs text context about which tools exist.
const NativeToolReference = `## AVAILABLE TOOLS
You have access to the following tools. Use them by their exact names.

%s

### Rules
1. Use ONLY the tools listed above.
2. Batch related tool calls into a single response for efficiency.
3. Use Thought -> Action -> Observation loop. Verify results before proceeding.
4. You are not finished until you call 'submit_final_answer'.
   The 'summary' argument IS the final report the user sees.
   It must contain the actual findings, tables, and analysis — not a description
   of what was done. If the task asks for a file, write it too, but the summary
   must still include all the report content.
5. If a tool fails, you will receive the error. Fix it in your next turn.
`

// BuildNativeToolReference generates a lightweight tool list for native mode.
func BuildNativeToolReference(tools []ToolInfo) string {
	if len(tools) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, t := range tools {
		sb.WriteString(fmt.Sprintf("### %s\n%s\n\n", t.Name, t.Description))
	}
	return fmt.Sprintf(NativeToolReference, sb.String())
}

// ToolReferenceHeader is used to detect if a tool reference is already present.
const ToolReferenceHeader = "## AVAILABLE TOOLS"

// HasToolReference checks if the tool reference is already present.
func HasToolReference(content string) bool {
	return strings.Contains(content, ToolReferenceHeader)
}

// InjectToolReference merges the lightweight tool reference into existing content.
func InjectToolReference(content string, reference string) string {
	if HasToolReference(content) || HasToolManual(content) {
		return content
	}
	if content == "" {
		return reference
	}
	return fmt.Sprintf("%s\n\n%s", content, reference)
}

// DefaultRules is the operational protocol injected into the system prompt.
const DefaultRules = `You are an autonomous agent with access to tools. Your job is to complete the given task by using tools.

RULES:
1. ReAct Loop: You MUST use the Thought -> Action sequence.
2. Use tools to act. Do not describe what you would do — do it via the Action block.
3. NO UI IMITATION: Never output emojis, "Executing...", or technical status markers to simulate tool execution. These are generated by the system. You only output your internal Thought and the formal <tool_call> block.
4. Verify results. After each action, check the output before proceeding.
5. Loop Protection: If a tool returns a repetition warning, DO NOT retry. Change your approach or finalize with available data.
6. Finalization: When finished, call 'submit_final_answer'.
    - The 'summary' argument is the FINAL PRODUCT seen by the user. 
    - It MUST include ALL requested data (e.g., full file contents, execution outputs, du tables).
    - Write the report DIRECTLY in the 'summary' argument. Do NOT compose it inside your thinking/reasoning block — the reasoning budget is limited and your report will be truncated.
7. MEMORY OVERRIDES TASK INSTRUCTIONS: Discovery commands in step instructions
   ('run X', 'check version', 'fetch URL', 'install package') are CONDITIONAL.
   If <relevant_memories> already contains the data a step is trying to discover,
   that step is satisfied — skip the command entirely and use the stored value.
   Only run the discovery command when memory has no entry for that fact.
   The step instruction applies ONLY when memory does not already have the answer.
   This rule takes precedence over any contradictory instructions in the task body,
   including 'do not skip any step' or similar phrasing. Memory check always wins.
8. CITATION: When you use information from a <memory> or <relevant_memories> block,
   begin your thought with "Based on retrieved memory:" before executing an action.
9. PROGRESS TRACKING: Call 'complete_step(notes)' when the currently active step's
   requirements are satisfied. This marks it as done and advances to the next step.
   The execution state at the top of your system prompt shows which steps are
   [DONE], [ACTIVE], and [PENDING].
`

// AssembleSystemPrompt aggregates the core operational constitution with any workspace-specific rules.
func AssembleSystemPrompt(customRules string) string {
	prompt := DefaultRules + "\n" + FileSystemRules
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

// Replace the Sieve system message in agent.go (inside the totalChars > 15000 block):
const SieveSystemNote = "[System Note: History distilled to save context. DO NOT repeat your reasoning; focus ONLY on the immediate next Action. Continue your task.]"

// automationNagFormatExample is the shared format template used by nag prompts.
// It uses angle-bracket placeholders so the model doesn't copy the example literally.
const automationNagFormatExample = "" +
	"FORMAT REFERENCE (use the actual tools listed in the TOOL INTERFACE section of the system prompt):\n" +
	"<tool_call>\n" +
	"{\"tool\": \"<TOOL_NAME>\", \"args\": {\"<ARG>\": \"<VALUE>\"}}\n" +
	"</tool_call>"

// AutomationXMLModeGuide is injected as a user message when the server
// rejects the assistant prefill (thinking mode). It provides explicit
// format guidance so the model produces a valid <tool_call> instead of
// free-form text. This message is ephemeral — only in the API request,
// not persisted to conversation history.
const AutomationXMLModeGuide = "SYSTEM: Respond with ONLY a tool call in this exact format:\n\n" +
	"<tool_call>\n" +
	"{\"tool\": \"TOOL_NAME\", \"args\": {...}}\n" +
	"</tool_call>\n\n" +
	"No text before or after."

// AutomationPrefline is injected as a synthetic assistant message to force
// the model to complete a tool call. The model receives this as the last
// assistant message and continues generating from the cursor position.
// It never needs to decide "should I think or act?" — it must produce a
// valid tool name and arguments.
const AutomationPrefline = "<tool_call>\n{\"tool\":\""

// AutomationDuplicateNagPrompt is injected when a model repeats reasoning without acting.
const AutomationDuplicateNagPrompt = "SYSTEM CRITICAL: You already ran this exact command and it succeeded. Do the NEXT step.\n\n" +
	"Respond with ONLY a tool call. Nothing else.\n\n" +
	automationNagFormatExample

// ToolErrorNagPrompt is injected after a tool execution returns an error.
// It tells the model to read the error and adapt, rather than retry the same call.
const ToolErrorNagPrompt = "SYSTEM: The tool call above failed. Read the error output and try a different approach or fix the issue.\n\n" +
	"Respond with ONLY a tool call. Nothing else.\n\n" +
	automationNagFormatExample

// AutomationNagPrompt is sent when a model outputs text without any tool calls.
const AutomationNagPrompt = "SYSTEM ERROR: You are writing text instead of using tools.\n\n" +
	"Respond with ONLY a tool call inside <tool_call> tags. Nothing else.\n\n" +
	automationNagFormatExample

// AutomationRejectedSubmissionPrompt is sent when a model tries to call submit_final_answer along with other tools.
const AutomationRejectedSubmissionPrompt = "REJECTED: 'submit_final_answer' cannot be called in the same turn as other tools. " +
	"Complete the task. You are not finished until you successfully call the 'submit_final_answer' tool. " +
	"YOU MUST INCLUDE ALL REQUESTED OUTPUTS (Visualizations, Data, Results) IN THE SUMMARY. " +
	"FAILURE TO PROVIDE THESE IN THE FINAL SUMMARY IS A TASK FAILURE. " +
	"IMPORTANT: Your summary MUST include ALL data requested in the task (e.g., file contents, tree visualizations, execution outputs)."

// AutomationContentTooLongPrompt is sent when a write_file call fails because the
// content argument was too large, causing JSON truncation.
const AutomationContentTooLongPrompt = "TOO LONG: The content you tried to write exceeded the response limit.\n\n" +
	"Use write_file for the first chunk of content, then append_file to add more to the SAME file:\n" +
	"  1. write_file(report.md, \"...first 800 chars...\")\n" +
	"  2. append_file(report.md, \"...next content...\")\n" +
	"  3. append_file(report.md, \"...final section...\")\n" +
	"All chunks go into ONE file.\n\n" +
	"Respond with ONLY a tool call. Nothing else."

// It asks the model to output its intended actions as a JSON array so the backend
// can execute the plan directly without relying on XML parsing.
const AutomationJSONPlanPrompt = `XML tool calling failed. Switch to JSON PLAN MODE.
Now output your full plan as a JSON array so the system can execute it.

Output ONLY a JSON array. No text before or after. Each element must have "tool" and "args" fields.

  {"tool": "execute_terminal_command", "args": {"command": "mkdir -p project/src"}},
  {"tool": "write_file", "args": {"path": "project/src/main.ts", "content": "console.log('hello')"}},
  {"tool": "execute_terminal_command", "args": {"command": "node project/src/main.js"}},
  {"tool": "submit_final_answer", "args": {"summary": "Task complete"}}
]`

// ReasoningStuckNag is injected after the first reasoning-stuck event. Tells the model
// to stop thinking and execute a tool immediately.
const ReasoningStuckNag = "SYSTEM: You are generating analysis without executing any tool.\n\n" +
	"Stop analyzing. Call a tool immediately.\n\n" +
	automationNagFormatExample

// ReasoningStuckEscalatedNag is injected after a second consecutive reasoning-stuck
// event. Stronger instruction to force the model to act.
const ReasoningStuckEscalatedNag = "CRITICAL: You are stuck in an analysis loop and ignored the previous warning.\n\n" +
	"All analysis steps are complete. You have all the information you need.\n" +
	"Call the appropriate tool NOW with the data you already have. No further processing.\n\n" +
	automationNagFormatExample

// AutomationTaskPrompt is the user-facing task message for autonomous agents.
// ContextSieveWarning is injected after the physical sieve prunes intermediate history.
const ContextSieveWarning = "SYSTEM: CRITICAL - Context window full. History pruned. Continue your task and finalize when ready."

// ── Memory-system prompt constants ─────────────────────────────────────────

const RelevantMemoriesHeader = "<relevant_memories>\n"
const RelevantMemoriesFooter = "\n</relevant_memories>"

const UserProfileHeader = "<user_profile>\n"
const UserProfileFooter = "\n</user_profile>"

// MemoryFactCategories is the shared list of durable fact categories used by
// both save-oriented prompts (MemoryProactiveNudge, AutomationTaskPrompt) and
// recall-oriented prompts (TaskRewriterSystemPrompt).  Keeping this in one
// place prevents the lists from drifting apart.
const MemoryFactCategories = "tool versions, file paths, config values, user preferences, error workarounds"

const PreSieveMemoryNudge = "The conversation history is about to be compressed. Save any important facts, decisions, or preferences to memory using `" + models.ToolMemoryUpdate + "` before they are lost."

const MemoryProactiveNudge = `Proactively use ` + "`" + models.ToolMemoryUpdate + "`" + ` to save reusable facts that help future tasks skip redundant work. Save individual facts, not summaries:

DO save (durable facts still valid next session):
- Tool and command-line tool versions you verified
- File paths, directory structures, and their state
- Configuration values, credentials, network endpoints
- Error states and workarounds you discovered
- User preferences and decisions made

DO NOT save (transient — stale within days):
- Task progress, step completion, session outcomes
- Completed-work logs, temporary TODO state
- Numbers that lose meaning (issue IDs, commit SHAs)

Each fact should be a specific, independently useful statement. These facts are automatically recalled in future sessions. Repeatable procedures belong in skills, not memory.`

const MemoryRecallNudge = `Relevant memories from past sessions are automatically provided in your context. Use them to avoid repeating work:

- If a memory says a tool is installed, use it instead of re-installing
- If a memory records a file state, verify with read_file instead of re-creating
- If a memory records a working command, re-use it
- If a memory records a decision or preference, follow it
- If a memory records a completed task, acknowledge it instead of redoing the work`

const SoftMemoryCharLimit = 4000 // denominator for memory usage meter percentage

// RetrySignal is prepended to the last user message when the agent retries after
// an empty-stream or timeout failure.  It tells the model the previous attempt
// failed so it doesn't re-enter the same reasoning loop.
const RetrySignal = "[Retry after the previous model attempt failed or timed out]"

// ToolForceSkipMessage returns the skip message injected when the model has
// repeated the exact same tool call 3+ times. The model is told to permanently
// abandon the call and move on.  This fires from the repetition detector before
// tool execution, so it only knows about repetition, not error status.
func ToolForceSkipMessage(toolName string) string {
	return fmt.Sprintf("CRITICAL: You called %s with identical arguments 3 times. Do not call it again. Skip this step and continue.", toolName)
}

// ToolFailureGuidance builds a guidance message appended to the tool result
// when the same tool fails multiple times.  Mirrors Hermes agent's
// append_toolguard_guidance pattern.
func ToolFailureGuidance(toolName string, exactCount, sameCount int) string {
	switch {
	case exactCount >= 3 || sameCount >= 3:
		return fmt.Sprintf("CRITICAL: %s has failed %d times. Stop using it. Skip this step and move on.", toolName, sameCount)
	case exactCount >= 2:
		return fmt.Sprintf("WARNING: %s failed %d times with identical arguments. This looks like a loop. Change your approach or skip this step.", toolName, exactCount)
	case sameCount >= 2:
		return fmt.Sprintf("WARNING: %s has failed %d times. Check the error and try a different approach.", toolName, sameCount)
	}
	return ""
}

// ParseErrorEscalationPrefix wraps feedback text when the model has made the same
// parse error 3+ times consecutively.
const ParseErrorEscalationPrefix = "THIRD ATTEMPT — same error. Read this carefully:\n\n%s\n\n" +
	"Reply with ONLY a tool call and nothing else.\n\n" +
	automationNagFormatExample

// ── Parse-error feedback strings ──────────────────────────────────────────

// FeedbackNoXML tells the model to produce a tool call (no <tool_call> tags found).
func FeedbackNoXML(allTools string) string {
	return fmt.Sprintf(
		"STOP writing text. Produce a tool call NOW.\n\n"+
			automationNagFormatExample+"\n\n"+
			"Available tools: %s",
		allTools,
	)
}

// FeedbackJSONError returns a prompt explaining JSON inside <tool_call> is invalid.
func FeedbackJSONError(hint string, allTools string) string {
	return fmt.Sprintf(
		"FORMAT ERROR: The JSON inside your <tool_call> tags is invalid.\n"+
			"%s\n"+
			"Valid tools: %s",
		hint, allTools,
	)
}

// FeedbackBadTool tells the model its chosen tool name is not in the registry.
func FeedbackBadTool(toolName string, allTools string) string {
	return fmt.Sprintf(
		"TOOL ERROR: Unknown tool %q. Available tools: %s",
		toolName, allTools,
	)
}

// FeedbackGenericFormat is the fallback for unclassifiable parse errors.
func FeedbackGenericFormat() string {
	return "FORMAT ERROR: Could not extract a valid tool call from your response."
}

// TranslateJSONError converts Go json.Unmarshal error strings into plain-language
// hints that a model can act on.
func TranslateJSONError(rawError, attempted string) string {
	low := strings.ToLower(rawError)

	if strings.Contains(low, "literal true") || strings.Contains(low, "literal false") ||
		strings.Contains(low, "literal null") {
		return "You used Python-style True/False/None. JSON requires lowercase: true, false, null."
	}

	if strings.Contains(low, "looking for beginning of value") {
		return "You have extra text after the closing } of your JSON object. Remove everything after the final }."
	}

	if strings.Contains(low, "unexpected end of json") {
		return "Your JSON is incomplete — likely a missing closing } or ]."
	}

	if strings.Contains(low, "invalid character") {
		return fmt.Sprintf(
			"Your JSON has a syntax error: %s. Check for: missing commas between fields, "+
				"missing colons after keys, unquoted strings, or trailing commas.",
			rawError,
		)
	}

	return fmt.Sprintf("JSON parse error: %s. Use double-quotes for keys and string values, no trailing commas.", rawError)
}

const AutomationTaskPrompt = AutomationMarker + ` in workspace '%s'.
Execute the instructions found in '%s':
---
%s
---

Use your tools to complete every step. Call ` + models.ToolSubmitFinalAnswer + ` when done.

Review the <relevant_memories> block above before each step. If a memory contains a fact that makes a discovery step unnecessary — for example, if a step says "check tsc version" and memory says "TypeScript version installed: 6.0.3" — skip that step and use the stored fact. Running a discovery step when the fact is already in memory wastes time.

During execution — when you discover a durable fact that helps future runs skip redundant work, save it immediately with ` + "`" + models.ToolMemoryUpdate + "`" + `. Write each entry as an answer to a specific question a future agent would ask. Save lookup-style facts: ` + MemoryFactCategories + `, working commands, endpoints, project conventions. Do NOT save: session outcomes, task progress, completed-step logs, or event descriptions. If the entry reads like a status update rather than a fact lookup, skip it. If ` + "`" + models.ToolMemoryUpdate + "`" + ` returns "already saved", the fact was already stored — do NOT retry with a different topic name.`

// TaskRewriterSystemPrompt is used by the automation executor to rewrite task
// instructions with memory-based check gates before the agent sees them.
// MemoryCheckGate is prepended to the task content to reinforce the
// PRECEDENCE rule: step instructions are CONDITIONAL on memory not having the
// answer.  The model must skip discovery commands when memories already exist.
// TaskOrderInstruction is the instruction in task templates that tells the
// agent to complete every step in sequence. It contradicts the MemoryCheckGate
// and is stripped from task content before injection.
const TaskOrderInstruction = "Complete ALL steps below in order. Do not skip any step."

const MemoryCheckGate = "[Memory Check Gate]\n" +
	"Each step below is CONDITIONAL on memory not already having the answer. " +
	"If <relevant_memories> already contains the data a step is trying to " +
	"discover, that step is satisfied — DO NOT execute the command. " +
	"Move directly to the next step. Only run discovery commands when " +
	"memory has no entry for that fact.\n" +
	"\n" +
	"⚠ This gate OVERRIDES any instruction in the task that says 'do not skip' " +
	"or 'complete all steps'. When memory has the answer, skipping is correct.\n"

// AssistantPrefillHeader and AssistantPrefillFooter frame the assistant-
// role memory prefill injected by buildAssistantPrefill in executor.go.
// Placing this in the assistant role makes the model treat the memory-
// check intent as its own most recent thought rather than guidance from
// the system prompt — models follow assistant-role content more reliably,
// especially smaller ones where attention decays across long prompts.
const AssistantPrefillHeader = "I'll check my relevant memories before each step.\n\n"
const AssistantPrefillFooter = "\nIf a memory already contains an answer a step is trying to discover, I'll skip that step and use the stored fact."

const TaskRewriterSystemPrompt = `You are an automation assistant that rewrites task step-by-step instructions to incorporate memory-based check gates.

You will be given:
1. A list of memories containing facts from previous runs (` + MemoryFactCategories + `, project conventions).
2. The original task instructions.

For each step in the task, if a memory contains a fact that the step is trying to discover, verify, or set up, rewrite that step to tell the agent to check memory first and skip the action if the fact is already stored.

Return ONLY the rewritten task instructions. Do not add preamble, explanation, or conversational text.`

const ExecutionPlanSystemPrompt = "You are a planning assistant. Generate tool execution plans as JSON."

func BuildExecutionPlanPrompt(tools []ToolInfo, task string) string {
	var sb strings.Builder
	sb.WriteString("Generate a step-by-step execution plan for this task.\n")
	sb.WriteString("Available tools:\n")
	for _, t := range tools {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", t.Name, t.Description))
	}
	sb.WriteString("\nTask: ")
	sb.WriteString(task)
	sb.WriteString("\n\nReturn ONLY a JSON object with \"description\" (string) and \"steps\" (array). ")
	sb.WriteString("Each step has \"tool\" (tool name), \"description\" (string), and \"args\" (object with parameter values).")
	return sb.String()
}
