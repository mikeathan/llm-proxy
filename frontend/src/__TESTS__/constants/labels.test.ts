import { describe, it, expect } from 'vitest'
import { activityLabel, helpersLabel } from '../../constants/labels'

// The one-line summary above an assistant answer (ChatBubble). Duration shows
// only when it was measured (a run watched live), never a guessed one.
describe('activityLabel', () => {
  it('says what a live run is doing', () => {
    expect(activityLabel('thinking', 0, 0)).toBe('Thinking')
    expect(activityLabel('working', 1, 4)).toBe('Working · 1 step')
    expect(activityLabel('generating', 3, 9)).toBe('Writing the answer')
  })

  it('sums up a finished run, with its duration when it was watched', () => {
    expect(activityLabel('done', 3, 42)).toBe('Worked 42s · 3 steps')
    expect(activityLabel('done', 3, 0)).toBe('3 steps')
    expect(activityLabel('done', 0, 7)).toBe('Worked 7s')
    expect(activityLabel('done', 0, 0)).toBe('Reasoning')
  })
})

// The helpers a turn used, counted in the footer; each one's model, outcome and tokens in the tooltip, since they are
// not in the turn's own token count.
describe('helpersLabel', () => {
  it('counts the helpers and lists what each used', () => {
    const label = helpersLabel([
      { agent: 'web-researcher', model: 'gpt', status: 'done', prompt_tokens: 900, completion_tokens: 120 },
      { agent: 'web-researcher', status: 'busy' },
    ])
    expect(label.text).toBe('2 helpers')
    expect(label.title).toContain('web-researcher on gpt: done')
    expect(label.title).toContain('120 tokens generated')
    expect(label.title).toContain('web-researcher: busy')
  })
  it('says one helper in the singular', () => {
    expect(helpersLabel([{ agent: 'web-researcher', status: 'done' }]).text).toBe('1 helper')
  })
})
