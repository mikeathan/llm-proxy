# Plan: Re-enable Tool-Call Grammar Constraint (opt-in, llama.cpp-safe)

**Status:** `proposed` — grammar constraint is currently **disabled** everywhere; this plan tracks re-enabling it correctly.
**Created:** 2026-09-05
**Last Updated:** 2026-09-05
**SPECs affected:** SPEC-001 (agent loop), SPEC-002 (tool-call parser)
**Subsystems:** cross-cutting (proxy/`tool_constraint.go`, assistant/`stream.go` `buildChatRequest`, model config)
**Supersedes (follow-up to):** `docs/PLANS/ARCHIVE/cross-cutting/llamacpp-grammar-constraint.md` (2026-06-12, archived "complete" — describes the original design that was since removed)

---

## 1. Context — why the grammar is OFF

The original design (archived plan, 2026-06-12) wired `GBNFConstraint` for
provider `"local"` in `buildChatRequest` **whenever native tools were active**,
so every tool-using turn against a locally-managed llama.cpp carried a
`grammar` field alongside the native `tools` array.

On 2026-09-05 this was found to be a guaranteed upstream rejection: modern
llama.cpp returns `400 "Cannot use custom grammar constraints with tools."`
for any request combining a custom grammar with native `tools`. An assistant
run against a locally-managed `Qwen3.5-35B-A3B` failed before generation with
exactly that error, while the same model registered as an OpenAI-style
endpoint (no grammar attached) worked. The wiring was removed
(`stream.go` `buildChatRequest` no longer sets `req.Grammar`; the
`providerConstraints` map is gone). The stale skill-doc section was corrected
in `docs/skills/agent-loop.md`.

**Current state (post-removal):**
- `proxy.GBNFConstraint` still exists and is unit-tested, but **no code path
  calls `Apply`** — no request ever carries a grammar.
- Tool-call argument validation is purely post-hoc (`handleContentToolCalls` +
  `ValidateToolCall`); native args are constrained server-side by llama.cpp's
  own tool template.

## 2. Why re-enable at all

Small local models in **XML text mode** (no native tools) still write free-text
`<tool_call>{…}</tool_call>` blocks and produce structurally broken JSON
(mismatched brackets, truncated objects). The original motivation (archived
plan §Problem) is unchanged: each broken call wastes a full turn. A
generation-time grammar would catch those at the token level — **if** it can be
applied to a request shape llama.cpp accepts.

## 3. Required work (the blockers)

Re-enabling is not a one-line flag — four pieces are needed:

1. **Envelope-aware grammar.** The current builder emits a *bare-JSON* grammar
   (`root ::= "{" … "}"`), which matches nothing the XML path produces (models
   write prose + a `<tool_call>` envelope). `buildGBNF` (or a new variant) must
   wrap the object grammar in the envelope, e.g.
   `root ::= "<tool_call>" ws "{" … "}" ws "</tool_call>"` — or the grammar is
   only usable on a dedicated JSON-only output path (nothing uses one today).
2. **Never with native `tools`.** llama.cpp 400s grammar + tools together.
   The grammar may only be attached when the request carries **no** native
   tools (`llmTools == nil`, i.e. the XML-text path /
   `computeNextResponseStreamXML`).
3. **Opt-in, not default.** Add an explicit per-model toggle (e.g.
   `grammar: true` on `ModelConfig`/`ModelOverride`, surfaced in model
   settings) — default **off**. Sending grammar by default to arbitrary
   OpenAI-compatible servers is unsafe: some ignore it (useless), strict
   validators 400 unknown params, and it is only meaningful on GBNF-capable
   engines (llama.cpp, some TGI/vLLM).
4. **Provider gate.** Only apply when the endpoint is a GBNF-capable local
   llama.cpp (provider `"local"`), matching condition 2.

## 4. Acceptance criteria

- A model with the opt-in flag set and running in XML text mode against local
  llama.cpp produces tool-call JSON that is structurally valid **or** the
  request is sent unchanged (constraint remains never-fatal — `Apply()`
  returning false must fall through, as before).
- No request carrying native `tools` ever includes `grammar` (regression test
  already exists: `TestBuildChatRequestSkipsGrammarWithNativeTools`).
- Default configuration is byte-identical to today (grammar absent).

## 5. Related references

- Removed wiring: `backend/internal/core/assistant/stream.go` `buildChatRequest`
- Builder: `backend/internal/core/proxy/tool_constraint.go` (`GBNFConstraint`)
- Skill doc (current behaviour): `docs/skills/agent-loop.md` → "Output Constraint (GBNF Grammar)"
- Original design: `docs/PLANS/ARCHIVE/cross-cutting/llamacpp-grammar-constraint.md`
- Incident (2026-09-05): assistant `400 Cannot use custom grammar constraints with tools.`
