# Antigravity Global Agent Directives

## Compliance Standards
- **Constitution Authority**: Every task must comply with `/CONSTITUTION.md`. If a task requires a violation, the agent must halt and request a formal override.
- **Spec-First Execution**: Before starting any task, agents MUST read the relevant file in `/docs/SPECS/` and `/docs/PLANS/`.
- **Pre-Flight Summary**: Every agent interaction must include a summary of the constraints understood from these documents before any code is generated.
- **Documentation Integrity**: When implementing new features or modifying existing critical logic, agents MUST update `/CONSTITUTION.md` and relevant design documents (SDDs/Blueprints) in `/docs/` to reflect the new system state. Documentation must never be out of sync with the implementation.

## Security & Tooling Rules
- **Network Access**: All network interaction is mediated through `NetworkTools` (see `/tools/network.go`). Raw I/O is prohibited.
- **Binary Metadata**: Extraction from GGUF files must use authorized parsing libraries. No filename-based regex extraction.
- **Context Handling**: `context.Context` is mandatory for all async/io operations.

## Prompting & Protocol
- **Single Source of Truth**: All systemic prompts, nag messages, and protocol instructions MUST be centralized in `internal/core/assistant/prompts/templates.go`. Hardcoded prompt strings in logic files are prohibited.
- **Agnostic Standard**: All tool calling must adhere to the `<tool_call>` standard defined in `templates.go`. Do not implement provider-specific tool calling logic.
17. 
18. ## Context Discipline ()
19. - **Targeted Execution**: Avoid large `list_directory` or `read_file` calls on entire repositories. If you receive a "Truncated Output" warning, you MUST use targeted tools like `grep`, `search_files`, or `read_range` to access specific content.
20. - **Submission Finality**: When context pressure is detected (SYSTEM Warning), prioritize finalizing your task and emitting `submit_final_answer` immediately. Do not perform extraneous cleanup or diagnostic turns when the window is nearly full.
21. - **Heuristic Awareness**: If you have already provided a comprehensive Markdown report but are unable to emit the JSON tool call due to technical constraints, the system will attempt to accept your text as a final submission. However, you should always strive to use the formal `submit_final_answer` tool as your primary exit point.