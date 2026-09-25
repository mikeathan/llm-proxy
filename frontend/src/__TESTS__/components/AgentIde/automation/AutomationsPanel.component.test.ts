import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import AutomationsPanel from '../../../../components/AgentIde/automation/AutomationsPanel.vue'
import type { Automation } from '../../../../types/dispatcher'

function automation(overrides: Partial<Automation> = {}): Automation {
  return {
    id: 'ws/nightly',
    workspace: 'ws',
    name: 'nightly',
    task_file: 'task.md',
    strategy: 'isolated',
    trigger: 'manual',
    ...overrides,
  }
}

function mountPanel(auto: Automation) {
  return mount(AutomationsPanel, {
    props: {
      groupedAutomations: { ws: [auto] },
      selectedAutomationId: undefined,
    },
    global: { stubs: { Icon: true, InlineConfirm: true } },
  })
}

// The backend serializes the scheduler admission as `queued` / `queue_position`
// (AutomationInfo). These tests fail if the panel starts reading a different
// key (e.g. `is_queued`), which would silently hide the badge and the cancel
// action.
describe('AutomationsPanel queued badge', () => {
  it('renders the queued position from the `queued` field', () => {
    const wrapper = mountPanel(automation({ queued: true, queue_position: 3 }))

    const badge = wrapper.find('.status-queued')
    expect(badge.exists()).toBe(true)
    expect(badge.text()).toContain('Queued #3')
  })

  it('offers the cancel action only for a queued automation', () => {
    const queued = mountPanel(automation({ queued: true, queue_position: 1 }))
    expect(queued.find('button[title="Cancel Queued Run"]').exists()).toBe(true)

    const running = mountPanel(automation({ is_running: true }))
    expect(running.find('button[title="Cancel Queued Run"]').exists()).toBe(false)
  })

  it('prefers the running indicator over the queued badge', () => {
    const wrapper = mountPanel(automation({ is_running: true, queued: true }))

    expect(wrapper.find('.status-running').exists()).toBe(true)
    expect(wrapper.find('.status-queued').exists()).toBe(false)
  })
})
