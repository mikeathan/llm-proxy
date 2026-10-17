import { describe, it, expect } from 'vitest'
import { chatModelOptions, chosenModel, DEFAULT_MODEL_VALUE } from '../../../utils/assistant/modelChoice'

describe('chatModelOptions', () => {
  it('leads with the default, named after the primary model', () => {
    const options = chatModelOptions(['a', 'b'], 'a')
    expect(options[0]).toEqual({ value: DEFAULT_MODEL_VALUE, label: 'Default (a)' })
    expect(options.slice(1).map((o) => o.value)).toEqual(['a', 'b'])
  })

  it('says plainly that no primary is set', () => {
    expect(chatModelOptions(['a'], '')[0]?.label).toBe('Default')
  })
})

describe('chosenModel', () => {
  it('is the stored model while the registry still has it', () => {
    expect(chosenModel('b', ['a', 'b'])).toBe('b')
  })

  it('falls back to the default for a model that was removed or never set', () => {
    expect(chosenModel('gone', ['a', 'b'])).toBe(DEFAULT_MODEL_VALUE)
    expect(chosenModel(DEFAULT_MODEL_VALUE, ['a'])).toBe(DEFAULT_MODEL_VALUE)
  })
})
