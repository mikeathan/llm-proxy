import type { ProfileDetail, ProfileInput } from '../../types/profile'

const ID_MAX_LENGTH = 40
const COPY_SUFFIX = ' (copy)'
const COPY_ID_SUFFIX = '-copy'

/** An id the server accepts, made from a name: lowercase letters, digits and dashes, at most 40. */
export function slugId(name: string): string {
  return name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+/, '').slice(0, ID_MAX_LENGTH).replace(/-+$/, '')
}

/** The id itself when free, else with -2, -3, … so a new agent never lands on an existing one. */
export function uniqueId(base: string, taken: readonly string[]): string {
  if (!base || !taken.includes(base)) return base
  for (let n = 2; ; n++) {
    const suffix = `-${n}`
    const candidate = base.slice(0, ID_MAX_LENGTH - suffix.length).replace(/-+$/, '') + suffix
    if (!taken.includes(candidate)) return candidate
  }
}

/** A new agent: a quiet, answer-first assistant that saves nothing on its own, until the person changes it. */
export function blankInput(): ProfileInput {
  return {
    name: '', summary: '', instructions: '', answer_first: true, may_ask: true, max_steps: 0, tools: [],
    memory: '', memory_save: 'model', reminders: false, delegates: [],
  }
}

export function inputFromDetail(detail: ProfileDetail): ProfileInput {
  return {
    name: detail.name, summary: detail.summary, instructions: detail.instructions, answer_first: detail.answer_first,
    may_ask: detail.may_ask, max_steps: detail.max_steps ?? 0, tools: [...(detail.tools ?? [])], memory: detail.memory ?? '',
    memory_save: detail.memory_save, reminders: detail.reminders, delegates: [...(detail.delegates ?? [])],
  }
}

/** A copy of an agent to edit under a new id. */
export function copyOf(detail: ProfileDetail, takenIds: readonly string[]): { id: string; input: ProfileInput } {
  return {
    id: uniqueId(slugId(`${detail.id}${COPY_ID_SUFFIX}`), takenIds),
    input: { ...inputFromDetail(detail), name: `${detail.name}${COPY_SUFFIX}` },
  }
}

export function sameInput(a: ProfileInput, b: ProfileInput): boolean {
  return JSON.stringify(a) === JSON.stringify(b)
}
