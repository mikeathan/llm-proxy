import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import MonitorPanel from '../../../../components/AgentIde/common/MonitorPanel.vue'

const METRICS = { total_executions: 7, successful: 5, failed: 2, skipped: 0, total_latency_ms: 0 }

function mountPanel(props: Record<string, unknown> = {}) {
  return mount(MonitorPanel, {
    props: { history: [], loading: false, metrics: METRICS, ...props },
    global: { stubs: { Icon: true } },
  })
}

describe('MonitorPanel', () => {
  it('shows run history and dispatcher metrics', () => {
    const wrapper = mountPanel()
    expect(wrapper.text()).toContain('Total Runs')
    expect(wrapper.text()).toContain('7')
  })

  it('shows assistant sessions and the lane blocker only when given', async () => {
    const wrapper = mountPanel()
    expect(wrapper.find('[data-test="assistant-activity"]').exists()).toBe(false)

    await wrapper.setProps({ assistantSessions: [], laneWaitingLabel: 'ws/nightly' })
    expect(wrapper.find('[data-test="assistant-activity"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('Waiting for ws/nightly to finish')
  })
})
