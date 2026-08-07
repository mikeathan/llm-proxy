---
status: reference
last_reviewed: 2026-07-19
---

# Audit: Hermes Agent does not structurally block report file writes

**Date**: 2026-07-19
**Severity**: note
**Subsystem**: docs (plan docs)

## Symptom

A weak local model (Qwen3.5-9B) called `write_file` to persist a network-scan report despite the system prompt instructing it not to. Earlier plan docs (`docs/PLANS/ARCHIVE/cross-cutting/universal-agent-completion.md:699`) cited "Hermes guardrail layers (for small/weak models)" raising the question of whether we should copy a Hermes structural guardrail that blocks file writes when not explicitly requested.

## Finding

Verified against Nous Research's actual Hermes Agent source and architecture docs:

- **Hermes has no structural guardrail** that blocks `write_file` (or any tool) based on task intent. It treats `write_file` as a normal tool like any other.
- The "guardrail layers" cited in the plan doc are exclusively **completion gate** mechanisms: think-block stripping, one-shot empty nudge, content-with-tools fallback, verification stop, exact-repeat detection. None are tool-use blocks.
- Hermes relies entirely on system prompt instructions ("do not write the report to a file") — same as llm-proxy. Both are vulnerable to weak models ignoring the instruction.
- The `content-with-tools` fallback in Hermes (`_last_content_with_tools`) captures assistant text when the model writes prose AND calls a tool in the same turn, using that text as fallback if the model goes silent next turn. This applies to housekeeping tools (todo, memory) not `write_file` specifically.

## Conclusion

No structural fix to port from Hermes. The gap is inherent to prompt-based tool governance. The `save_file`→`write_file` rename was cosmetic (aligns naming with Hermes/OpenAI conventions) and does not change model behavior. If file-write prevention is needed for weak models, a guardrail layer beyond Hermes' design would be required.

## References

- Hermes Agent source: `run_agent.py` (AIAgent), `agent/prompt_builder.py`
- Hermes docs: https://hermes-agent.nousresearch.com/docs/developer-guide/agent-loop
- Plan doc: `docs/PLANS/ARCHIVE/cross-cutting/universal-agent-completion.md` §699
