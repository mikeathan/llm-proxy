// Centralized mapping of user-facing UI copy / description strings.
//
// RULE: UI text that varies by state (phase, status, …) is defined here and
// referenced by name — never inline in templates or composables. This keeps a
// single source of truth for copy so wording, casing and punctuation stay
// consistent and searchable. (Symbols/emojis live in constants/icons.ts.)
import type { HelperProgress, SubagentRun } from '../types/assistant'
import type { InsetPhase } from '../types/inset'
import { formatElapsedSeconds } from '../utils/format/time'
import { formatTokenCount } from '../utils/format/units'

// HELPER_STATUS_LABEL says how a delegated helper stands when it has no step to show (ToolCallSegment).
export const HELPER_STATUS_LABEL: Record<HelperProgress['status'], string> = {
  running: 'Starting',
  done: 'Done',
  busy: 'Not run: every helper slot was busy',
  refused: 'Not run',
  failed: 'Failed',
  stopped: 'Stopped',
}

// activityLabel is the one-line summary above an assistant answer
// (ChatBubble): what a live run is doing, or what a finished one did. `steps`
// counts tool calls; `seconds` is the measured run time — live, or from the
// turn's stored run record — and 0 when there is none (older sessions); never
// guessed.
const ACTIVITY_SEPARATOR = ' · '
const plural = (n: number, word: string) => `${n} ${word}${n === 1 ? '' : 's'}`

export function activityLabel(phase: InsetPhase, steps: number, seconds: number): string {
  switch (phase) {
    case 'thinking':   return 'Thinking'
    case 'working':    return `Working${ACTIVITY_SEPARATOR}${plural(steps, 'step')}`
    case 'generating': return 'Writing the answer'
    default: {
      const parts = [seconds > 0 ? `Worked ${formatElapsedSeconds(seconds)}` : '', steps > 0 ? plural(steps, 'step') : ''].filter(Boolean)
      return parts.length ? parts.join(ACTIVITY_SEPARATOR) : 'Reasoning'
    }
  }
}

// The tokens a turn used, as the provider reported them: what it generated
// in the text, with the prompt tokens it processed (the context, re-counted on
// every call of an agent loop) in the tooltip.
export function tokenUsageLabel(promptTokens: number, completionTokens: number): { text: string; title: string } {
  const generated = `${formatTokenCount(completionTokens)} tokens`
  return { text: generated, title: `${generated} generated · ${formatTokenCount(promptTokens)} prompt tokens processed` }
}

// The helpers (delegated sub-agents) a turn used: counted in the footer, with each one's model, outcome and tokens in
// the tooltip — they are not in the turn's own token count.
const HELPERS_TITLE_SEPARATOR = '\n'
export function helpersLabel(runs: SubagentRun[]): { text: string; title: string } {
  const lines = runs.map((r) => {
    const who = r.model ? `${r.agent} on ${r.model}` : r.agent
    const tokens = r.completion_tokens ? ` · ${tokenUsageLabel(r.prompt_tokens ?? 0, r.completion_tokens).title}` : ''
    return `${who}: ${r.status}${tokens}`
  })
  return { text: plural(runs.length, 'helper'), title: lines.join(HELPERS_TITLE_SEPARATOR) }
}

// Upstream-retry notice copy — event-driven, surfaced as an inline notice
// while a transient upstream failure is being retried.
export const UPSTREAM_RETRYING_TEMPLATE = 'Upstream retrying ({attempt}/{max}) — {reason}'
export const STUCK_RESTART_NOTICE = 'The model got stuck thinking — trying again'
export const MODEL_STARTING_NOTICE = 'Model is starting — waiting for it to become ready'
// A remote proxy refused to serve this request because a run is using the model
// it would have to stop. Not a fault: the run is protected on purpose.
export const MODEL_BUSY_NOTICE = 'Model in use by a running job — this request was not served'
// Wait / Cancel prompt shown while the client waits for a busy local model to
// free up. The backend keeps waiting until the user cancels (or the run ends).
export const MODEL_BUSY_DIALOG_TITLE = 'Local model is busy'
export const MODEL_BUSY_DIALOG_MESSAGE =
  'The local model is serving another run. This request is waiting for it to free up.'
export const MODEL_BUSY_WAIT_LABEL = 'Wait'
export const MODEL_BUSY_CANCEL_LABEL = 'Cancel'
export const TRANSPORT_ERROR_LABEL = 'transport error'
