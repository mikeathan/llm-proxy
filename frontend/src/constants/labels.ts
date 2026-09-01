// Centralized mapping of user-facing UI copy / description strings.
//
// RULE: UI text that varies by state (phase, status, …) is defined here and
// referenced by name — never inline in templates or composables. This keeps a
// single source of truth for copy so wording, casing and punctuation stay
// consistent and searchable. (Symbols/emojis live in constants/icons.ts.)
import type { InsetPhase } from '../types/inset'

// getPhaseLabel maps an InsetPhase to the assistant-bubble header label.
// `toolCount` is interpolated for the working/done states, which describe how
// many tool calls ran this turn. Unknown/idle falls back to the bare "Assistant".
export function getPhaseLabel(phase: InsetPhase, toolCount = 0): string {
  switch (phase) {
    case 'thinking':   return 'Assistant — thinking...'
    case 'working':    return `Assistant — working (${toolCount} tools)`
    case 'generating': return 'Assistant — generating answer...'
    case 'done':       return `Assistant — ${toolCount} tools · completed`
    default:           return 'Assistant'
  }
}

// Upstream-retry notice copy — event-driven, surfaced as an inline notice
// while a transient upstream failure is being retried.
export const UPSTREAM_RETRYING_TEMPLATE = 'Upstream retrying ({attempt}/{max}) — {reason}'
export const MODEL_STARTING_NOTICE = 'Model is starting — waiting for it to become ready'
export const TRANSPORT_ERROR_LABEL = 'transport error'
