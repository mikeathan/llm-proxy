import { describe, expect, it } from 'vitest'
import { filterModels, groupModelsByKey, modelStatus, orderModels, providerSummary, railEntries, routingRole } from '../../domain/models'
import type { Model } from '../../types/model'

const model = (m: Partial<Model>): Model => ({ name: 'm', provider: 'local', endpoint: '', active: false, ready: false, ...m })
const IDLE = model({ name: 'qwen', filename: 'Qwen3.8-27B.gguf' })
const RUNNING = model({ name: 'llama', active: true, ready: true })
const LOADING = model({ name: 'mistral', active: true, ready: false })
const CLOUD = model({ name: 'gpt', provider: 'openai', model_id: 'gpt-5-mini' })

describe('modelStatus', () => {
  it('says whether a local model runs, loads or idles, and calls a cloud model cloud', () => {
    expect(modelStatus(RUNNING)).toEqual({ state: 'success', label: 'Running' })
    expect(modelStatus(LOADING)).toEqual({ state: 'running', label: 'Loading' })
    expect(modelStatus(IDLE)).toEqual({ state: 'neutral', label: 'Idle' })
    expect(modelStatus(CLOUD)).toEqual({ state: 'info', label: 'Cloud' })
  })
})

describe('orderModels', () => {
  it('puts the running model first, then local, then cloud, keeping the order within each', () => {
    expect(orderModels([CLOUD, IDLE, RUNNING, model({ name: 'b' })]).map((m) => m.name)).toEqual(['llama', 'qwen', 'b', 'gpt'])
  })
})

describe('filterModels', () => {
  const all = [IDLE, RUNNING, CLOUD]
  it('narrows by scope', () => {
    expect(filterModels(all, 'local', '').map((m) => m.name)).toEqual(['qwen', 'llama'])
    expect(filterModels(all, 'cloud', '').map((m) => m.name)).toEqual(['gpt'])
    expect(filterModels(all, 'all', '')).toHaveLength(3)
  })
  it('matches the name, the file or the model id, without case', () => {
    expect(filterModels(all, 'all', '27B').map((m) => m.name)).toEqual(['qwen'])
    expect(filterModels(all, 'all', 'MINI').map((m) => m.name)).toEqual(['gpt'])
    expect(filterModels(all, 'all', '  ')).toHaveLength(3)
  })
})

describe('routingRole', () => {
  it('names the primary and the fallback, and nothing else', () => {
    const config = { primary_model: 'gpt', fallback_model: 'qwen' }
    expect(routingRole(CLOUD, config)).toBe('Primary')
    expect(routingRole(IDLE, config)).toBe('Fallback')
    expect(routingRole(RUNNING, config)).toBeNull()
    expect(routingRole(RUNNING, undefined)).toBeNull()
  })
})

const KEYED = (name: string, provider: string, key?: string) => model({ name, provider: provider as Model['provider'], provider_config: key ? { api_key_name: key } : undefined })

describe('groupModelsByKey', () => {
  it('makes one group per key, in the keys\' order, even one with no model yet', () => {
    const groups = groupModelsByKey([KEYED('a', 'nvidia', 'work'), KEYED('b', 'nvidia', 'work'), KEYED('c', 'nvidia', 'home')], ['home', 'work', 'spare'])
    expect(groups.map((g) => [g.key, g.models.map((m) => m.name)])).toEqual([['home', ['c']], ['work', ['a', 'b']], ['spare', []]])
  })

  it('puts a model whose key is gone or unnamed in a last group with no key', () => {
    const groups = groupModelsByKey([KEYED('a', 'nvidia', 'old'), KEYED('b', 'nvidia')], ['work'])
    expect(groups.map((g) => [g.key, g.models.map((m) => m.name)])).toEqual([['work', []], ['', ['a', 'b']]])
  })

  it('has no group at all for a provider with neither keys nor models', () => {
    expect(groupModelsByKey([], [])).toEqual([])
  })
})

describe('providerSummary', () => {
  it('says not set up until there is a key, then counts keys and models', () => {
    expect(providerSummary(0, 0)).toBe('Not set up')
    expect(providerSummary(1, 1)).toBe('1 key · 1 model')
    expect(providerSummary(2, 0)).toBe('2 keys · 0 models')
  })
})

describe('railEntries', () => {
  it('lists All models, This computer, then each cloud provider with what it holds', () => {
    const entries = railEntries(
      [{ id: 'openrouter', label: 'OpenRouter' }, { id: 'gemini', label: 'Google Gemini' }],
      [IDLE, RUNNING, KEYED('x', 'openrouter', 'dev')],
      (p) => (p === 'openrouter' ? 1 : 0),
    )
    expect(entries.map((e) => [e.id, e.label, e.summary, e.state])).toEqual([
      ['all', 'All models', '3 models', 'neutral'],
      ['local', 'This computer', '2 models', 'success'],
      ['openrouter', 'OpenRouter', '1 key · 1 model', 'info'],
      ['gemini', 'Google Gemini', 'Not set up', 'neutral'],
    ])
  })

  it('marks a provider whose key failed its test, even though it is connected', () => {
    const entries = railEntries([{ id: 'openrouter', label: 'OpenRouter' }], [], () => 1, (p) => p === 'openrouter')
    expect(entries[2]).toMatchObject({ summary: '1 key · 0 models · key test failed', state: 'error' })
  })

  it('shows This computer as idle when no local model runs', () => {
    expect(railEntries([], [IDLE], () => 0)[1]?.state).toBe('neutral')
  })
})
