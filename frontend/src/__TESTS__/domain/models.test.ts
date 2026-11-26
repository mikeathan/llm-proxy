import { describe, expect, it } from 'vitest'
import { filterModels, modelStatus, orderModels, routingRole } from '../../domain/models'
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
