import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '../../../router'

const getGlobalActiveRunsMock = vi.fn()
const promoteQueuedRunMock = vi.fn()
const cancelQueuedRunMock = vi.fn()

vi.mock('../../../services/assistant/assistantService', () => ({
  AssistantService: {
    getGlobalActiveRuns: (...args: unknown[]) => getGlobalActiveRunsMock(...args),
    promoteQueuedRun: (...args: unknown[]) => promoteQueuedRunMock(...args),
    cancelQueuedRun: (...args: unknown[]) => cancelQueuedRunMock(...args),
  },
}))
const confirm = vi.fn()
vi.mock('../../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm }) }))

import RunActivityPill from '../../../components/layout/RunActivityPill.vue'
import { useGlobalRunActivity } from '../../../composables/assistant/useGlobalRunActivity'

const LANES = [
  { lane: 'local', limit: 1, running: 1, waiting: 0, holder_keys: ['ws/a'] },
  { lane: 'cloud', limit: 3, running: 0, waiting: 0, holder_keys: [] },
]
const IDLE_PAYLOAD = { lanes: [{ ...LANES[0], running: 0, holder_keys: [] }, LANES[1]] }

const ACTIVE_PAYLOAD = {
  lane_holders: [
    { key: 'ws/a', kind: 'automation', workspace_id: 'ws', automation: 'a', label: 'ws/a', since: '2026-09-20T00:00:00Z' },
  ],
  queued: [
    { key: 'ws/b', lane: 'cloud', workspace_id: 'ws', automation: 'b', label: 'ws/b', position: 2, queued_at: '2026-09-20T00:00:00Z' },
  ],
  lanes: LANES,
}

// One external caller waiting for the local model, plus an ordinary queued
// automation: only the former gets operator actions.
const INBOUND_PAYLOAD = {
  lane_holders: [],
  queued: [
    { key: 'inbound:1', kind: 'inbound', lane: 'local', workspace_id: '', label: 'model-b', model: 'model-b', position: 1, queued_at: '2026-09-20T00:00:00Z' },
    { key: 'ws/b', kind: 'automation', lane: 'cloud', workspace_id: 'ws', automation: 'b', label: 'ws/b', position: 1, queued_at: '2026-09-20T00:00:00Z' },
  ],
  lanes: IDLE_PAYLOAD.lanes,
}

const pillOf = (w: VueWrapper) => w.find('button[aria-controls="run-activity-panel"]')
const panelOf = (w: VueWrapper) => w.find('#run-activity-panel')

// Rows of running work are links, so the pill needs the app router like the real header.
async function mountPill() {
  const router = createAppRouter(createMemoryHistory())
  await router.push('/overview')
  const w = mount(RunActivityPill, { global: { plugins: [router], stubs: { Icon: true } } })
  await vi.waitFor(() => expect(pillOf(w).exists()).toBe(true))
  return w
}

async function openPanel() {
  const w = await mountPill()
  await pillOf(w).trigger('click')
  await flushPromises()
  return w
}

describe('RunActivityPill', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    confirm.mockResolvedValue(true)
  })

  it('renders nothing until the first answer arrives', () => {
    getGlobalActiveRunsMock.mockReturnValue(new Promise(() => {}))
    const w = mount(RunActivityPill, { global: { stubs: { Icon: true, RouterLink: true } } })
    expect(pillOf(w).exists()).toBe(false)
    w.unmount()
  })

  it('rests as a quiet idle indicator that still shows lane capacity', async () => {
    getGlobalActiveRunsMock.mockResolvedValue(IDLE_PAYLOAD)
    const w = await mountPill()
    expect(pillOf(w).attributes('data-state')).toBe('idle')
    expect(pillOf(w).attributes('aria-label')).toBe('No runs active — show run lanes')
    await pillOf(w).trigger('click')
    const meters = panelOf(w).findAll('[role="meter"]')
    expect(meters.map((m) => [m.attributes('aria-label'), m.attributes('aria-valuenow'), m.attributes('aria-valuemax')])).toEqual([
      ['Local lane', '0', '1'],
      ['Cloud lane', '0', '3'],
    ])
    w.unmount()
  })

  it('summarises running and queued runs, then lists them by lane on click', async () => {
    getGlobalActiveRunsMock.mockResolvedValue(ACTIVE_PAYLOAD)
    const w = await mountPill()

    const pill = pillOf(w)
    expect(pill.attributes('data-state')).toBe('running')
    expect(pill.text()).toContain('1 running')
    expect(pill.text()).toContain('1 queued')
    expect(pill.attributes('aria-expanded')).toBe('false')
    expect(panelOf(w).exists()).toBe(false)

    await pill.trigger('click')
    expect(pill.attributes('aria-expanded')).toBe('true')
    const panel = panelOf(w)
    expect(panel.attributes('role')).toBe('dialog')

    const local = panel.get('[data-lane="local"]')
    expect(local.get('[role="meter"]').attributes('aria-valuenow')).toBe('1')
    expect(local.text()).toContain('ws/a')
    expect(local.text()).toContain('Automation')
    // Queued runs sit in their lane with their 1-based scheduler position.
    const cloud = panel.get('[data-lane="cloud"]')
    expect(cloud.text()).toContain('ws/b')
    expect(cloud.text()).toContain('#2')
    w.unmount()
  })

  it('closes on Escape and from its close button', async () => {
    getGlobalActiveRunsMock.mockResolvedValue(ACTIVE_PAYLOAD)
    const w = await openPanel()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await nextTick()
    expect(panelOf(w).exists()).toBe(false)

    await pillOf(w).trigger('click')
    await w.get('button[aria-label="Close run activity"]').trigger('click')
    expect(panelOf(w).exists()).toBe(false)
    w.unmount()
  })

  it('reports unavailable instead of stale counts when a poll fails', async () => {
    getGlobalActiveRunsMock.mockResolvedValue(ACTIVE_PAYLOAD)
    const w = await mountPill()
    await vi.waitFor(() => expect(pillOf(w).text()).toContain('1 running'))

    // A later poll fails: the last known lists must not read as live state.
    getGlobalActiveRunsMock.mockRejectedValue(new Error('connection refused'))
    await useGlobalRunActivity().refresh()
    await nextTick()

    const pill = pillOf(w)
    expect(pill.attributes('data-state')).toBe('unavailable')
    expect(pill.text()).toContain('Run state unavailable')
    expect(pill.text()).not.toContain('running')

    await pill.trigger('click')
    expect(w.text()).toContain('reach the run scheduler')
    expect(w.text()).toContain('connection refused')
    // Neither the stale lists nor the stale lane bars are shown.
    expect(panelOf(w).find('[data-lane]').exists()).toBe(false)
    w.unmount()
  })
})

// An external /v1 caller waiting for a local model is the one queued row the
// operator can act on: `kind: inbound` (or the minted `inbound:` key) marks it.
describe('RunActivityPill inbound callers', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    confirm.mockResolvedValue(true)
    getGlobalActiveRunsMock.mockResolvedValue(INBOUND_PAYLOAD)
    promoteQueuedRunMock.mockResolvedValue(undefined)
    cancelQueuedRunMock.mockResolvedValue(undefined)
  })

  const button = (w: VueWrapper, name: string) =>
    w.findAll('button').find((b) => b.text() === name || b.attributes('aria-label') === name)

  it('lists waiting external callers apart, and offers only them the operator actions', async () => {
    const w = await openPanel()
    const external = w.get('[data-group="external"]')
    expect(external.text()).toContain('model-b')
    expect(button(w, 'Serve model-b now')).toBeDefined()
    expect(button(w, 'Dismiss model-b')).toBeDefined()
    // A run queued for lane concurrency is not the operator's call to make.
    expect(w.get('[data-lane="cloud"]').findAll('button')).toHaveLength(0)
    // The consequence is stated, not implied.
    expect(external.text()).toContain('cancels that run')
    w.unmount()
  })

  it('serves a waiting caller after confirming, and refreshes the lists', async () => {
    const w = await openPanel()
    const callsBefore = getGlobalActiveRunsMock.mock.calls.length

    confirm.mockResolvedValueOnce(false)
    await button(w, 'Serve model-b now')!.trigger('click')
    await flushPromises()
    expect(promoteQueuedRunMock).not.toHaveBeenCalled()

    await button(w, 'Serve model-b now')!.trigger('click')
    await flushPromises()
    expect(confirm).toHaveBeenCalledWith(expect.objectContaining({ type: 'warning' }))
    expect(promoteQueuedRunMock).toHaveBeenCalledWith('inbound:1')
    expect(getGlobalActiveRunsMock.mock.calls.length).toBeGreaterThan(callsBefore)
    w.unmount()
  })

  it('dismisses a waiting caller without serving it', async () => {
    const w = await openPanel()
    await button(w, 'Dismiss model-b')!.trigger('click')
    await flushPromises()
    expect(cancelQueuedRunMock).toHaveBeenCalledWith('inbound:1')
    expect(promoteQueuedRunMock).not.toHaveBeenCalled()
    w.unmount()
  })

  it('a failed action leaves the panel usable', async () => {
    promoteQueuedRunMock.mockRejectedValue(new Error('no queued caller with that key'))
    const w = await openPanel()
    await button(w, 'Serve model-b now')!.trigger('click')
    await flushPromises()
    // Nothing thrown, and the row is actionable again (the busy guard cleared).
    expect(button(w, 'Serve model-b now')!.attributes('disabled')).toBeUndefined()
    w.unmount()
  })

  describe('running work links', () => {
    const SINCE = '2026-09-20T00:00:00Z'
    const PAYLOAD = {
      lane_holders: [
        { key: 'chat:ws', kind: 'interactive', workspace_id: 'ws', label: 'chat:ws', conversation_id: 'conv-9', since: SINCE },
        { key: 'ws/a', kind: 'automation', workspace_id: 'ws', automation: 'a', label: 'ws/a', since: SINCE },
      ],
      queued: [
        { key: 'ws/b', kind: 'automation', lane: 'local', workspace_id: 'ws', automation: 'b', label: 'ws/b', position: 1, queued_at: SINCE },
        { key: 'inbound:1', kind: 'inbound', lane: 'local', workspace_id: '', label: 'model-b', model: 'model-b', position: 2, queued_at: SINCE },
      ],
      lanes: [
        { lane: 'local', limit: 2, running: 2, waiting: 0, holder_keys: ['chat:ws', 'ws/a'] },
        { lane: 'cloud', limit: 3, running: 0, waiting: 0, holder_keys: [] },
      ],
    }
    const linkTo = (w: VueWrapper, text: RegExp) => panelOf(w).findAll('a').find((a) => text.test(a.text()))

    it('links a running chat to its conversation, named by its workspace', async () => {
      getGlobalActiveRunsMock.mockResolvedValue(PAYLOAD)
      const w = await openPanel()
      const a = linkTo(w, /Chat in ws/)
      expect(a, 'the running chat is a link').toBeDefined()
      expect(a!.attributes('href')).toBe('/workspaces/ws/assistant/conv-9')
      expect(a!.attributes('aria-label')).toBe('Open Chat in ws')
      expect(panelOf(w).text()).not.toContain('chat:ws')
      w.unmount()
    })

    it('links running and queued automations to their pages', async () => {
      getGlobalActiveRunsMock.mockResolvedValue(PAYLOAD)
      const w = await openPanel()
      for (const label of [/^ws\/a$/, /^ws\/b$/]) {
        const a = linkTo(w, label)
        expect(a, `${label} is a link`).toBeDefined()
        expect(decodeURIComponent(a!.attributes('href')!)).toBe(`/automations/${label.source.slice(1, -1).replace('\\/', '/')}`)
      }
      w.unmount()
    })

    it('leaves an external API caller as plain text with its serve / dismiss actions', async () => {
      getGlobalActiveRunsMock.mockResolvedValue(PAYLOAD)
      const w = await openPanel()
      expect(panelOf(w).text()).toContain('model-b')
      expect(linkTo(w, /model-b/)).toBeUndefined()
      expect(panelOf(w).find('button[aria-label="Serve model-b now"]').exists()).toBe(true)
      w.unmount()
    })

    it('closes the panel when a link is followed', async () => {
      getGlobalActiveRunsMock.mockResolvedValue(PAYLOAD)
      const w = await openPanel()
      await linkTo(w, /Chat in ws/)!.trigger('click')
      await flushPromises()
      expect(panelOf(w).exists()).toBe(false)
      w.unmount()
    })

    it('links runs outside any lane (the group shown when no lane summary is reported)', async () => {
      getGlobalActiveRunsMock.mockResolvedValue({ lane_holders: PAYLOAD.lane_holders, queued: PAYLOAD.queued, lanes: [] })
      const w = await openPanel()
      expect(linkTo(w, /Chat in ws/)).toBeDefined()
      expect(linkTo(w, /^ws\/b$/)).toBeDefined()
      w.unmount()
    })
  })
})
