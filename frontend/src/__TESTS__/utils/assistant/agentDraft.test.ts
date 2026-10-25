import { describe, expect, it } from 'vitest'
import { blankInput, copyOf, inputFromDetail, sameInput, slugId, uniqueId } from '../../../utils/assistant/agentDraft'
import type { ProfileDetail } from '../../../types/profile'

const research: ProfileDetail = {
  id: 'research', name: 'Research', summary: 'Finds and cites', source: 'builtin', instructions: 'Cite.', answer_first: true,
  may_ask: true, memory_save: 'model', reminders: false, delegates: ['web-researcher'], default_for: [], delegating_to: [], replaces_builtin: false,
}

describe('agentDraft', () => {
  it('makes an id the server accepts from a name', () => {
    expect(slugId('My Research Helper!')).toBe('my-research-helper')
    expect(slugId('  --Ünïcode  & more--')).toBe('n-code-more')
    expect(slugId('x'.repeat(60))).toHaveLength(40)
    expect(slugId('!!!')).toBe('')
  })

  it('finds a free id', () => {
    expect(uniqueId('scout', ['research'])).toBe('scout')
    expect(uniqueId('scout', ['scout', 'scout-2'])).toBe('scout-3')
    expect(uniqueId('x'.repeat(40), ['x'.repeat(40)])).toHaveLength(40)
    expect(uniqueId('', [])).toBe('')
  })

  it('copies an agent under a new id and name', () => {
    const copy = copyOf(research, ['research', 'research-copy'])
    expect(copy.id).toBe('research-copy-2')
    expect(copy.input.name).toBe('Research (copy)')
    expect(copy.input.delegates).toEqual(['web-researcher'])
    expect(copy.input.delegates).not.toBe(research.delegates)
  })

  it('tells an edited draft from the saved one', () => {
    const saved = inputFromDetail(research)
    expect(sameInput(saved, inputFromDetail(research))).toBe(true)
    expect(sameInput(saved, { ...saved, summary: 'changed' })).toBe(false)
    expect(blankInput().name).toBe('')
  })

  it('keeps a local-only agent local-only when it is edited or copied', () => {
    const local: ProfileDetail = { ...research, local_only: true }
    expect(inputFromDetail(local).local_only).toBe(true)
    expect(copyOf(local, []).input.local_only).toBe(true)
    expect(inputFromDetail(research).local_only).toBe(false)
    expect(blankInput().local_only).toBe(false)
  })
})
