import { describe, it, expect } from 'vitest'
import { chatModelOptions, chosenModel, DEFAULT_MODEL_VALUE } from '../../../utils/assistant/modelChoice'
import type { Model } from '../../../types/model'

const model = (name: string, provider: string) => ({ name, provider }) as Model
const label = (p: string) => (p === 'opencode' ? 'OpenCode Go' : p === 'nvidia' ? 'NVIDIA NIM' : p)

describe('chatModelOptions', () => {
  it('leads with the default, named after the primary model and its provider', () => {
    const options = chatModelOptions([model('a', 'opencode'), model('b', 'local')], 'a', label)
    expect(options[0]).toEqual({ value: DEFAULT_MODEL_VALUE, label: 'Default (a · OpenCode Go)' })
    expect(options.slice(1).map((o) => o.value)).toEqual(['b', 'a'])
  })

  it('groups each model under where it runs, local first then providers by name, so the name stays short', () => {
    const options = chatModelOptions([model('deepseek-v4.1-flash', 'opencode'), model('deepseek-v4.1-flash-nvidia', 'nvidia'), model('Qwen3.6', 'local')], '', label)
    expect(options.slice(1).map((o) => [o.label, o.group])).toEqual([['Qwen3.6', 'Local'], ['deepseek-v4.1-flash-nvidia', 'NVIDIA NIM'], ['deepseek-v4.1-flash', 'OpenCode Go']])
    expect(options[0]?.group).toBeUndefined()
  })

  it('says plainly that no primary is set', () => {
    expect(chatModelOptions([model('a', 'local')], '', label)[0]?.label).toBe('Default')
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
