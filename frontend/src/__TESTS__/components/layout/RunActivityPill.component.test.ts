import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises, type DOMWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'

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

import RunActivityPill from '../../../components/layout/RunActivityPill.vue'
import { useGlobalRunActivity } from '../../../composables/assistant/useGlobalRunActivity'

const ACTIVE_PAYLOAD = {
  lane_holders: [
    { key: 'ws/a', kind: 'automation', workspace_id: 'ws', automation: 'a', label: 'ws/a', since: '2026-09-20T00:00:00Z' },
  ],
  queued: [
    { key: 'ws/b', lane: 'cloud', workspace_id: 'ws', automation: 'b', label: 'ws/b', position: 2, queued_at: '2026-09-20T00:00:00Z' },
  ],
}

// One external caller waiting for the local model, plus an ordinary queued
// automation: only the former gets operator actions.
const INBOUND_PAYLOAD = {
  lane_holders: [],
  queued: [
    {
      key: 'inbound:1',
      kind: 'inbound',
      lane: 'local',
      workspace_id: '',
      label: 'model-b',
      model: 'model-b',
      position: 1,
      queued_at: '2026-09-20T00:00:00Z',
    },
    {
      key: 'ws/b',
      kind: 'automation',
      lane: 'cloud',
      workspace_id: 'ws',
      automation: 'b',
      label: 'ws/b',
      position: 1,
      queued_at: '2026-09-20T00:00:00Z',
    },
  ],
}

describe('RunActivityPill', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders nothing while no run is active', async () => {
    getGlobalActiveRunsMock.mockResolvedValue({})

    const wrapper = mount(RunActivityPill)
    await flushPromises()

    expect(wrapper.find('.run-pill').exists()).toBe(false)
    wrapper.unmount()
  })

  it('summarises running and queued runs, then lists them on click', async () => {
    getGlobalActiveRunsMock.mockResolvedValue(ACTIVE_PAYLOAD)

    const wrapper = mount(RunActivityPill)
    await vi.waitFor(() => {
      expect(wrapper.find('.run-pill').exists()).toBe(true)
    })

    const pill = wrapper.find('.run-pill')
    expect(pill.text()).toContain('1 running')
    expect(pill.text()).toContain('1 queued')
    expect(pill.attributes('aria-expanded')).toBe('false')
    expect(pill.attributes('aria-controls')).toBe('run-activity-panel')
    expect(wrapper.find('.run-panel').exists()).toBe(false)

    await pill.trigger('click')

    expect(pill.attributes('aria-expanded')).toBe('true')
    expect(wrapper.find('.run-panel').exists()).toBe(true)
    expect(wrapper.text()).toContain('ws/a')
    expect(wrapper.text()).toContain('Automation')
    // Queued runs are listed with their 1-based scheduler position.
    expect(wrapper.text()).toContain('#2')
    expect(wrapper.text()).toContain('Cloud')

    wrapper.unmount()
  })

  it('dismisses the panel when Escape is pressed', async () => {
    getGlobalActiveRunsMock.mockResolvedValue(ACTIVE_PAYLOAD)

    const wrapper = mount(RunActivityPill)
    await vi.waitFor(() => {
      expect(wrapper.find('.run-pill').exists()).toBe(true)
    })
    await wrapper.find('.run-pill').trigger('click')
    expect(wrapper.find('.run-panel').exists()).toBe(true)

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await nextTick()

    expect(wrapper.find('.run-panel').exists()).toBe(false)
    wrapper.unmount()
  })

  it('reports unavailable instead of stale counts when a poll fails', async () => {
    getGlobalActiveRunsMock.mockResolvedValue(ACTIVE_PAYLOAD)

    const wrapper = mount(RunActivityPill)
    await vi.waitFor(() => {
      expect(wrapper.find('.run-pill').text()).toContain('1 running')
    })

    // A later poll fails: the last known lists must not read as live state.
    getGlobalActiveRunsMock.mockRejectedValue(new Error('connection refused'))
    const { refresh } = useGlobalRunActivity()
    await refresh()
    await nextTick()

    const pill = wrapper.find('.run-pill')
    expect(wrapper.find('.run-pill--stale').exists()).toBe(true)
    expect(pill.text()).toContain('Run state unavailable')
    expect(pill.text()).not.toContain('running')

    await pill.trigger('click')

    expect(wrapper.text()).toContain('reach the run scheduler')
    expect(wrapper.text()).toContain('connection refused')
    // The stale running/queued lists are not shown while state is unknown.
    expect(wrapper.find('.panel-list').exists()).toBe(false)

    wrapper.unmount()
  })
})

// An external /v1 caller waiting for a local model is the one queued row the
// operator can act on: `kind: inbound` (or the minted `inbound:` key) marks it.
describe('RunActivityPill inbound callers', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getGlobalActiveRunsMock.mockResolvedValue(INBOUND_PAYLOAD)
    promoteQueuedRunMock.mockResolvedValue(undefined)
    cancelQueuedRunMock.mockResolvedValue(undefined)
  })

  async function openPanel() {
    const wrapper = mount(RunActivityPill)
    await vi.waitFor(() => {
      expect(wrapper.find('.run-pill').exists()).toBe(true)
    })
    await wrapper.find('.run-pill').trigger('click')
    await flushPromises()
    return wrapper
  }

  // The inbound row carries one "Serve now" and one "Dismiss" button; a missing
  // one is a failure worth stating, not an index into undefined.
  function actionButton(row: DOMWrapper<Element>, index: number): DOMWrapper<Element> {
    const button = row.findAll('.row-action')[index]
    if (!button) throw new Error(`expected an action button at index ${index}`)
    return button
  }

  async function openInboundRow() {
    const wrapper = await openPanel()
    const row = wrapper.findAll('.panel-row').find((r) => r.text().includes('External'))
    if (!row) throw new Error(`expected an inbound row, got: ${wrapper.text()}`)
    return { wrapper, row }
  }

  it('tags a waiting external caller and offers only it the operator actions', async () => {
    const wrapper = await openPanel()

    const rows = wrapper.findAll('.panel-row')
    expect(rows).toHaveLength(2)

    const inbound = rows.find((r) => r.text().includes('External'))
    const queued = rows.find((r) => r.text().includes('Cloud'))
    if (!inbound || !queued) {
      throw new Error(`expected an External row and a Cloud row, got: ${wrapper.text()}`)
    }
    expect(inbound.text()).toContain('model-b')
    expect(inbound.findAll('.row-action')).toHaveLength(2)
    // A run queued for lane concurrency is not the operator's call to make.
    expect(queued.findAll('.row-action')).toHaveLength(0)
    // The consequence is stated, not implied.
    expect(wrapper.text()).toContain('cancels that run')

    wrapper.unmount()
  })

  it('serves a waiting caller and refreshes the lists', async () => {
    const { wrapper, row } = await openInboundRow()
    const callsBefore = getGlobalActiveRunsMock.mock.calls.length

    await actionButton(row, 0).trigger('click')
    await flushPromises()

    expect(promoteQueuedRunMock).toHaveBeenCalledWith('inbound:1')
    expect(getGlobalActiveRunsMock.mock.calls.length).toBeGreaterThan(callsBefore)

    wrapper.unmount()
  })

  it('dismisses a waiting caller without serving it', async () => {
    const { wrapper, row } = await openInboundRow()

    await actionButton(row, 1).trigger('click')
    await flushPromises()

    expect(cancelQueuedRunMock).toHaveBeenCalledWith('inbound:1')
    expect(promoteQueuedRunMock).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  it('a failed action leaves the panel usable', async () => {
    promoteQueuedRunMock.mockRejectedValue(new Error('no queued caller with that key'))
    const { wrapper, row } = await openInboundRow()

    await actionButton(row, 0).trigger('click')
    await flushPromises()

    // Nothing thrown, and the row is actionable again (the busy guard cleared).
    expect(actionButton(row, 0).attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })
})
