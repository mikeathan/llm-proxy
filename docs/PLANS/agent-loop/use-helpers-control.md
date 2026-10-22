# Use Helpers — a composer control that makes a research chat delegate

**Status:** proposed (2026-10-22). Awaiting the user's sign-off on the decisions in §6; nothing is built.
**Related Specs:** SPEC-001 (Agent Loop, §15 Delegation), SPEC-003 (UI), SPEC-012 (Prompt Profiles).
**Constitution refs:** II.13 (prompt text only in `templates.go`), II.3 (nothing leaves the machine unintended — see §4).
**Follows:** [`multi-agent-delegation.md`](multi-agent-delegation.md) (B7 first findings), PR #119 (the helper list in the tool schema).

## 1. Problem

Helpers are built, deployed and switched on, but a local model never uses them on its own. A research chat on vertex's
Qwen (16k window) ran ten searches and fetches itself, filled its window and lost the thread, although `delegate_task`
was offered the whole time.

The recorded first request of that turn was replayed against vertex's Qwen, five trials per variant
(`multi-agent-delegation.md`, "B7 first findings"):

| Variant | Delegated |
|---|---|
| As deployed | 0/5 |
| Guidance in the Research profile (three wordings) | 0/5 |
| The user asks in the message ("Use delegate_task to hand the angles to helpers") | **5/5** |

So the model delegates exactly when it is told to, for this turn, by the user. Profile text does not do it. What is
missing is a way to tell it that is not "remember to type a sentence every time".

## 2. Goal

A visible switch in the composer, **Use helpers**, that asks the assistant to hand the independent parts of the
message to helpers. Off by default. The user stays in control, the stored conversation keeps exactly what they typed,
and nothing is guessed about their intent.

## 3. Design

### 3.1 What the model is told (backend)

- The chat request gains `use_helpers: bool`, beside `model` and `profile`.
- When it is set, and the run can delegate (the profile names helpers and the sub-runner is enabled), the **last user
  message sent to the model** gets a short note appended. This is the pattern `withLocalTime` already uses: the note
  rides in the model's copy of the history only (`llmHistory` in `conversationService.Execute`), so the saved
  conversation and what the chat shows stay as typed, and the note is current on every turn.
- Wording lives in `prompts/templates.go` (II.13), in the same "Context for this message, not written by the user"
  form as the local-time note. Draft: *The user switched on helpers for this message. Hand each independent part of the
  task to a helper with `delegate_task`, one task per part, each written to stand alone; then answer from their
  summaries.*
- If the switch is on but the run cannot delegate, nothing is added and nothing fails.

### 3.2 What the UI knows (one source of truth)

The frontend must not re-derive "is a cloud helper model set". The backend answers:

- New `GET /admin/api/helpers` → `{ "available": bool, "model": "deepseek-v4.1-flash", "agents": [{id, name, summary}] }`.
  `available` is the runner's `Enabled()` (a cloud model is named and the lane classifier agrees). It is the
  endpoint the later Settings → Agents editor needs too.
- The profile list already returns each profile's `delegates` (the handler embeds `profile.Profile`); the frontend type
  `PromptProfile` just lacks the field.

### 3.3 The control (frontend)

- A third row in the composer's existing **Run settings** popover (`RunSettings.vue`), under Profile and Model.
- Shown when the selected profile has `delegates`. Enabled when `available`; otherwise shown disabled with the reason
  ("Pick a Helper model in Settings → Runs").
- When on, the Run settings button says so (`Research · helpers`), so it is not forgotten.
- `RunChoice` gains `use_helpers`; `sendMessage` posts it. It is sticky for the open conversation and off for a new
  one (D1).

### 3.4 Mock-up

```
 composer                                                              
 ┌─────────────────────────────────────────────────────────────────┐   
 │ Compare the latest llama.cpp, Ollama and vLLM releases…         │   
 └─────────────────────────────────────────────────────────────────┘   
  [ Research · helpers ˄ ]                                      ( ▶ )  
        │
        ▼ (opens above)
 ┌ Run settings ───────────────────────────────┐
 │ Profile                                     │
 │ [ Research                              ▾ ] │
 │ Finds, reads and cites; keeps what a source │
 │ says apart from what it concludes.          │
 │                                             │
 │ Model                                       │
 │ [ Qwen3.6 35B A3B (default)             ▾ ] │
 │                                             │
 │ Helpers                                     │
 │ [●] Use helpers for this chat               │
 │ Splits the question into parts and hands    │
 │ each to a helper on deepseek-v4.1-flash,    │
 │ which searches the web in parallel.         │
 │ Only what your assistant writes into the    │
 │ tasks leaves this machine.                  │
 └─────────────────────────────────────────────┘

 when no Helper model is set:   [○] Use helpers for this chat   (disabled)
                                Pick a Helper model in Settings → Runs.
 when the profile has no helpers (General): the row is not shown.
```

## 4. Privacy and cost

- Helpers run on a cloud model; the text the assistant writes into each task goes there. The control's help text says
  so, and names the model. Workspace notes (AGENTS.md) are not sent (shipped in #117).
- Helper tokens are on the turn's run record and the "N helpers" footer, not in the turn's own count.

## 5. Phases

| Phase | What | Accept |
|---|---|---|
| P0 | **Evidence for the note.** Replay the recorded request with the exact note form (not the typed sentence), 5 trials, on Qwen3.6; also two small or odd cases ("what is 2+2", a one-line lookup) with the note on, to see it does not delegate silly work; also one other local model on vertex. If the soft note is below 4/5, test forcing the first step with `tool_choice` on the delegate tool. | A table in this plan; the wording chosen from it |
| P1 | **Backend, test first.** `use_helpers` on the request, a context value, the note in `templates.go`, applied to `llmHistory`; `GET /admin/api/helpers`; SPEC-001 §15 and SPEC-003. | Chat-service test: the note reaches the model, the saved session is unchanged, nothing is added when the run cannot delegate |
| P2 | **Frontend.** `PromptProfile.delegates`, `RunChoice.use_helpers`, a `useHelpers` composable, the third row and the button state, labels in `constants/labels.ts`, tests. | `npm test`, `npm run lint`, `npm run build`, `npm run test:visual` |
| P3 | **Live on vertex** after the user deploys: the Go research question as a second question, switch on. | Helpers run; the answer arrives without hitting the window; the footer shows the helpers |

## 6. Decisions for the user

- **D1: how long the switch lasts.** (a) Sticky for the open conversation, off for a new one — **recommended**:
  research is a series of follow-ups, and the button shows it is on. (b) Per message: safest, but easy to forget.
  (c) Always on for a profile: no control, but then it cannot be turned off for a quick question.
- **D2: when helpers are not set up.** (a) Show the row disabled with the reason — **recommended**: the feature is
  discoverable. (b) Hide it.
- **D3: what the switch does.** (a) A note on the message, the model decides how to split — **recommended**, and what
  P0 measures. (b) Force the first step to be a delegate call (`tool_choice`): certain, but it cannot answer a trivial
  question directly, so only if P0 shows the note is unreliable.

## 7. Not in this plan

Choosing which helper (there is one), a Settings → Agents editor (Part C), pipelines, and showing "helpers were on but
not used" (a possible later addition once P3 shows how often it happens).

## Remaining Work

P0–P3, after the decisions in §6.
