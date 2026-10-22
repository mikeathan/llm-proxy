// Centralized mapping of user-facing UI copy / description strings.
//
// RULE: UI text that varies by state (phase, status, …) is defined here and
// referenced by name — never inline in templates or composables. This keeps a
// single source of truth for copy so wording, casing and punctuation stay
// consistent and searchable. (Symbols/emojis live in constants/icons.ts.)
import type { HelperProgress, SubagentRun } from '../types/assistant'
import type { HelpersProblem } from '../types/helpers'
import type { InsetPhase } from '../types/inset'
import { formatElapsedSeconds } from '../utils/format/time'
import { formatTokenCount } from '../utils/format/units'

// The composer's "Use helpers" row (RunSettings). The model is named because helpers run on a cloud model: what the
// assistant writes into each task goes there, and nothing else from this machine.
export const HELPERS_FIELD_LABEL = 'Helpers'
export const HELPERS_TOGGLE_LABEL = 'Use helpers for this chat'
export const HELPERS_ON_SUFFIX = 'helpers'
export function helpersHint(model: string): string {
  return `Splits the question into parts and hands each to a helper on ${model}, which searches the web in parallel. Only what your assistant writes into the tasks leaves this machine.`
}
// The row's help text and its link to Settings → Runs, by what is wrong with the helper model (or nothing: it can run).
// A model removed from the models is told apart from one that is not a cloud model, so the fix named is the right one.
export function helpersHelp(problem: HelpersProblem, model: string): { hint: string; link: string } {
  switch (problem) {
    case 'missing': return { hint: `The Helper model ${model} is no longer in your models.`, link: 'Pick another in Settings → Runs' }
    case 'not_cloud': return { hint: `The Helper model ${model} is not a cloud model, so it cannot run helpers.`, link: 'Pick a cloud model in Settings → Runs' }
    case 'unset': return { hint: 'Helpers run on a cloud model, and none is chosen yet.', link: 'Pick a Helper model in Settings → Runs' }
    default: return { hint: helpersHint(model), link: 'Change the Helper model in Settings → Runs' }
  }
}
export const HELPER_MODEL_UNAVAILABLE_SUFFIX = ' (not available)'

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
