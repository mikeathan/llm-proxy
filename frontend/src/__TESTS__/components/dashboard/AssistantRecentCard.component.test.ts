import { beforeEach, describe, expect, it, vi } from 'vitest'
import { RouterLinkStub, flushPromises, mount } from '@vue/test-utils'
import { memoryStorage } from '../../helpers/memoryStorage'

const assistant = vi.hoisted(() => ({ listSessions: vi.fn() }))
vi.mock('../../../services/assistant/assistantService', () => ({ AssistantService: assistant }))

import AssistantRecentCard from '../../../components/dashboard/cards/AssistantRecentCard.vue'

const session = (id: string, minutesAgo: number, over: Record<string, unknown> = {}) => ({ id, snippet: `about ${id}`, updated_at: new Date(Date.now() - minutesAgo * 60_000).toISOString(), ...over })
const mountCard = async () => {
  const w = mount(AssistantRecentCard, { global: { stubs: { RouterLink: RouterLinkStub } } })
  await flushPromises()
  return w
}

beforeEach(() => {
  vi.clearAllMocks()
  const storage = memoryStorage()
  storage.setItem('admin-ui:last-workspace', JSON.stringify({ version: 1, data: 'ws' }))
  vi.stubGlobal('localStorage', storage)
})

describe('AssistantRecentCard', () => {
  it('lists the latest conversations newest first, each a link into it, and says which is running', async () => {
    assistant.listSessions.mockResolvedValue([session('old', 90), session('new', 2, { running: true })])
    const w = await mountCard()
    const rows = w.findAll('[data-test="assistant-recent-session"]')
    expect(rows.map((r) => r.text())).toEqual([expect.stringContaining('about new'), expect.stringContaining('about old')])
    expect(rows[0]!.text()).toContain('Running')
    expect(w.get('[data-test="assistant-recent-count"]').text()).toContain('2 conversations · 1 running')
    expect(w.findAllComponents(RouterLinkStub)[0]!.props('to')).toMatchObject({ params: { ws: 'ws' } })
  })

  it('says there are none yet, and says when they could not be read', async () => {
    assistant.listSessions.mockResolvedValue([])
    expect((await mountCard()).text()).toContain('No conversations yet')
    assistant.listSessions.mockRejectedValue(new Error('down'))
    expect((await mountCard()).text()).toContain('could not be read')
  })
})
