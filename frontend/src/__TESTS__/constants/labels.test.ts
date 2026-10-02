import { describe, it, expect } from 'vitest'
import { activityLabel } from '../../constants/labels'

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
