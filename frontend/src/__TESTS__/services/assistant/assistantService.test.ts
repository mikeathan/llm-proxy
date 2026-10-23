import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { AssistantService } from '../../../services/assistant/assistantService'

// What the service puts on the wire. Tests of the callers mock this service, so only this one sees the request body:
// "use_helpers" was once built by the caller and then dropped here, so the switch worked on the page and never reached
// the server.
describe('AssistantService.sendMessage', () => {
  const fetchMock = vi.fn()
  beforeEach(() => {
    fetchMock.mockReset()
    fetchMock.mockResolvedValue({ ok: true, json: async () => ({ status: 'running', conversation_id: 'c1' }) })
    vi.stubGlobal('fetch', fetchMock)
  })
  afterEach(() => vi.unstubAllGlobals())

  const bodyOf = (): Record<string, unknown> => JSON.parse(fetchMock.mock.lastCall![1].body as string)

  it('sends use_helpers when the message asked for helpers', async () => {
    await AssistantService.sendMessage({ workspace_id: 'ws', message: 'compare A and B', profile: 'research', use_helpers: true })
    expect(bodyOf()).toMatchObject({ workspace_id: 'ws', message: 'compare A and B', profile: 'research', use_helpers: true })
  })

  it('sends nothing about helpers when it did not ask', async () => {
    await AssistantService.sendMessage({ workspace_id: 'ws', message: 'hello' })
    expect(bodyOf()).not.toHaveProperty('use_helpers')
  })

  it('sends a one-message agent only when a turn command named one', async () => {
    await AssistantService.sendMessage({ workspace_id: 'ws', message: 'compare A and B', agent: 'research' })
    expect(bodyOf()).toMatchObject({ message: 'compare A and B', agent: 'research' })
    await AssistantService.sendMessage({ workspace_id: 'ws', message: 'hello' })
    expect(bodyOf()).not.toHaveProperty('agent')
  })

  it('keeps sending the model and the profile only when chosen', async () => {
    await AssistantService.sendMessage({ workspace_id: 'ws', message: 'hello', model: 'qwen', profile: 'builder' })
    expect(bodyOf()).toMatchObject({ model: 'qwen', profile: 'builder' })
    await AssistantService.sendMessage({ workspace_id: 'ws', message: 'hello' })
    expect(bodyOf()).not.toHaveProperty('model')
    expect(bodyOf()).not.toHaveProperty('profile')
  })
})
