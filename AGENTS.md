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