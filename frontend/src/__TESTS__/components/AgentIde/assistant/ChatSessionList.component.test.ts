import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { memoryStorage } from '../../../helpers/memoryStorage'
import type { SessionBrief } from '../../../../types/assistant'

const today = new Date().toISOString()
const SESSIONS: SessionBrief[] = [
  { id: 'c1', snippet: 'Plan the NAS migration', updated_at: today },
  { id: 'c2', snippet: 'Summarise the logs', updated_at: today, running: true },
  { id: 'h1', snippet: 'From telegram', updated_at: today, source: 'webhook-telegram' },
]

async function mountList(sessions: SessionBrief[] = SESSIONS, workspaceId = 'ws') {
  // Pins persist through usePersistedState; a fresh module per test keeps the
  // singleton's state from leaking between tests.
  vi.resetModules()
  const { default: ChatSessionList } = await import('../../../../components/AgentIde/assistant/ChatSessionList.vue')
  const handlers = { onLoad: vi.fn(), onDelete: vi.fn(), onRename: vi.fn(), onNewChat: vi.fn(), onCancel: vi.fn() }
  const w = mount(ChatSessionList, {
    props: { sessions, currentSessionId: 'c1', workspaceId },
    attrs: handlers,
    global: { stubs: { Icon: true } },
  })
  const button = (name: RegExp) => w.findAll('button').find((b) => name.test(b.attributes('aria-label') ?? '') || name.test(b.text()))
  const headings = () => w.findAll('[data-test="session-group"]').map((g) => g.text())
  return { w, button, headings, ...handlers }
}

describe('ChatSessionList', () => {
  beforeEach(() => {
    vi.stubGlobal('localStorage', memoryStorage())
  })

  it('groups conversations, marks the open one and a running one in words', async () => {
    const { w, headings } = await mountList()
    expect(headings()).toEqual(['Today', 'Webhook'])
    expect(w.get('[aria-current="true"]').text()).toContain('Plan the NAS migration')
    expect(w.text()).toContain('Running')
  })

  it('opens a conversation, and starts a new one', async () => {
    const { button, onLoad, onNewChat } = await mountList()
    await button(/^summarise the logs/i)!.trigger('click')
    expect(onLoad).toHaveBeenCalledWith('c2')
    await button(/^new chat$/i)!.trigger('click')
    expect(onNewChat).toHaveBeenCalled()
  })

  it('searches conversation titles', async () => {
    const { w, headings } = await mountList()
    await w.get('input[type="search"]').setValue('nas')
    expect(headings()).toEqual(['Today'])
    expect(w.text()).not.toContain('Summarise the logs')
    await w.get('input[type="search"]').setValue('nothing like this')
    expect(w.text()).toMatch(/no conversation matches/i)
  })

  it('pins a conversation to the top, per workspace, and remembers it', async () => {
    const first = await mountList()
    await first.button(/^pin summarise the logs$/i)!.trigger('click')
    expect(first.headings()[0]).toBe('Pinned')
    expect(first.button(/^unpin summarise the logs$/i)).toBeDefined()

    // A fresh page load reads the stored pins back; another workspace has its own.
    const again = await mountList()
    expect(again.headings()[0]).toBe('Pinned')
    const other = await mountList(SESSIONS, 'ws-2')
    expect(other.headings()[0]).toBe('Today')
  })

  it('names every row action after its conversation', async () => {
    const { button, onDelete, onCancel } = await mountList()
    await button(/^delete plan the nas migration$/i)!.trigger('click')
    expect(onDelete).toHaveBeenCalledWith('c1')
    await button(/^stop summarise the logs$/i)!.trigger('click')
    expect(onCancel).toHaveBeenCalledWith('c2')
    expect(button(/^rename plan the nas migration$/i)).toBeDefined()
  })

  it('renames inline', async () => {
    const { w, button, onRename } = await mountList()
    await button(/^rename plan the nas migration$/i)!.trigger('click')
    const input = w.get('input[aria-label="Conversation title"]')
    await input.setValue('NAS move')
    await input.trigger('keydown', { key: 'Enter' })
    expect(onRename).toHaveBeenCalledWith('c1', 'NAS move')
  })

  it('says when the workspace has no conversation yet', async () => {
    const { w } = await mountList([])
    expect(w.text()).toMatch(/no conversations yet/i)
    expect(w.find('input[type="search"]').exists()).toBe(false)
  })
})
