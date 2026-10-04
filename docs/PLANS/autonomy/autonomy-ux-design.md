---
status: proposed
date: 2026-10-04
last_reviewed: 2026-10-04
related_specs: [SPEC-003, SPEC-009]
constitution_references: [V.2]
related_plans: [autonomy-roadmap.md, ../assistant-ui/assistant-automation-workbench-layout.md, ../cross-cutting/frontend-redesign-retro.md]
---

# Autonomy UX Design (design-only plan, milestone M0)

**Status:** proposed. **This plan produces designs, not code.** It exists so the autonomy features are
designed *together* once — and then each feature plan builds its own screens against these designs,
instead of five plans inventing five different patterns.

## Why a separate design plan

The autonomy features (agenda, attention policy, cross-channel, goals, consolidation) all put the
assistant in front of you **without being asked**. If that experience is confusing — "why did it message
me?", "where do I stop this?", "what is it doing right now?" — users switch autonomy off. The core design
problem is **trust**, and it spans all five features:

| Question the user will ask | Where the design must answer it |
|---|---|
| "What is the assistant going to do, and when?" | Today view — upcoming agenda, active goals |
| "Why did it do / say that?" | `RunReason` shown on every proactive run and message |
| "How do I stop it?" | One-click pause/snooze/cancel everywhere, including from Telegram |
| "Is it waiting for me?" | Pending approvals and questions, top of the Today view and in Telegram |
| "What did I miss?" | Digest and held notices |
| "What does it know about me?" | Consolidation review inbox, linked to memory |

## Ground truth (verified, 2026-10-04)

| Fact | Source |
|---|---|
| Top-level destinations today: Overview, Workspaces (with assistant, files, security, sections), Automations, Models, Activity, Settings; plus a bare `/design` primitives gallery | `frontend/src/router/index.ts:24-59`; `frontend/src/views/design/PrimitivesGallery.vue` |
| Existing notification surfaces: toasts, a notification dot, run notifications, app banner | `frontend/src/components/ui/Toast.vue`; `frontend/src/components/common/NotificationDot.vue`; `frontend/src/components/common/display/RunNotifications.vue`; `frontend/src/components/ui/AppBanner.vue` |
| Theme/token system and visual regression tests exist (retro redesign) | `../cross-cutting/frontend-redesign-retro.md`; `npm run test:visual` |
| A workbench layout redesign is proposed and needs its own sign-off (chat+editor split, automation master-detail) | `../assistant-ui/assistant-automation-workbench-layout.md` |
| Guardrail approvals render today as an in-chat prompt in the web UI only | `backend/internal/core/assistant/agent.go:416-470` (event `guardrail_blocked`) |

## Scope — what to design

### 1. Information architecture
- Decide: new top-level **Today** destination vs extending **Overview**. (Recommendation: *Today* as the
  new landing page; Overview keeps system health.)
- Where goals live: global list with a workspace filter, since a goal runs in a workspace but is "yours".

### 2. Screens (low-fi wireframes, then hi-fi in the existing theme)
| Screen | Content |
|---|---|
| **Today** | Waiting for you (approvals, questions) · Next up (agenda, next 24 h) · Active goals (status, last progress, next check) · Held for digest · Quiet-hours state |
| **Agenda** | List + calendar-strip view; filter by owner (you / agent / goal); create reminder; edit/snooze/cancel |
| **Goal detail** | Outcome and success criteria · limits (spend, ticks, tools, network) · journal timeline · next check · pause/resume/abandon · "mark achieved" |
| **New goal** | Guided form: outcome → how to know it's done → limits → workspace/model → first check time |
| **Attention settings** | Quiet hours (per weekday) · daily ping budget · urgency rules · digest times · channel preference · autonomous spend cap |
| **Channels** | Linked owner channels (Telegram chat), verification flow, per-channel mute |
| **Consolidation review** | "Learned today" proposals: add / merge / demote / delete, accept-all-low-risk, diff view of a merged fact |
| **Why panel** | Reusable component: shows `RunReason` + the policy decision for any run or notice |

### 3. Telegram message design (UX, not just transport)
- Formats for: reminder, follow-up result, goal progress, goal question, approval request (with
  Approve / Deny / Always allow buttons), digest, consolidation summary.
- Commands: `/today`, `/pause <goal>`, `/snooze 1h`, `/mute until 8am`, `/new` (start a fresh thread).
- Length limits and how a long result links back to the web UI.

### 4. Key flows (storyboards)
1. "Remind me Friday at 9 to call the bank" — from web chat and from Telegram.
2. Agent schedules its own follow-up; you see it on Today; it runs; result arrives as one notice.
3. A goal tick hits a blocked tool at 23:30 (quiet hours) → what happens, what you see in the morning.
4. Approve from Telegram; the same approval disappears from the web UI.
5. Conversation started on the phone, continued at the desk.
6. Morning digest; consolidation review.

### 5. Copy and states
Empty states, error states ("Telegram unreachable — held 3 notices"), and plain-language explanations of
every limit.

## Decisions for the user

| # | Decision | Recommendation |
|---|---|---|
| U1 | Today as new landing page, or a panel in Overview? | New landing page |
| U2 | Design artefact format | Low-fi wireframes as images/markdown under `docs/design/autonomy/` (new); hi-fi as Vue prototypes on the `/design` route, using real theme tokens |
| U3 | Combine with the workbench-layout sign-off or keep separate? | Keep separate, but review both in one session so navigation and layout agree |

## Phases

### Phase 0 — Inventory and IA
- Inventory existing components to reuse (toasts, notification dot, banner, run notifications, forms).
- IA decision (U1) and navigation sketch.
- **Acceptance:** an IA diagram and component inventory in the new `docs/design/autonomy/` folder, signed off.

### Phase 1 — Low-fi wireframes and storyboards
- All screens in §2, all flows in §4, Telegram formats in §3.
- **Acceptance:** user sign-off on each flow.

### Phase 2 — Hi-fi prototypes on `/design`
- Static Vue prototypes with real tokens and mock data; no backend.
- **Acceptance:** `npm test && npm run build && npm run test:visual` green; user sign-off.

### Phase 3 — Contracts out of design
- Draft **SPEC-011** sections for the UI-facing contracts (Today API shape, `RunReason` display fields,
  notice decision record) and the SPEC-003 amendment.
- **Acceptance:** drafts reviewed; `./scripts/check-agent-harness.sh` green after adding them.

## Risks and non-goals
- **Designing ahead of reality** — prototypes use mock data shaped by the draft contracts; each feature
  plan may adjust, and must update the design notes when it does.
- **Non-goal:** voice UI; mobile app. Telegram is the mobile surface.

## Remaining Work
Phases 0–3. Blocks the UI phases of every autonomy feature plan (backend phases can start in parallel).
