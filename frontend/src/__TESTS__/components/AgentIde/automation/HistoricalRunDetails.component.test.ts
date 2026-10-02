import { describe, it, expect, afterEach } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '../../../../router'
import { useConfirm } from '../../../../composables/ui/useConfirm'
import HistoricalRunDetails from '../../../../components/AgentIde/automation/HistoricalRunDetails.vue'
import type { AutomationRun } from '../../../../types/dispatcher'

const RUN: AutomationRun = {
  id: 'run_0001abcdef', workspace_id: 'ws', automation_name: 'nightly', timestamp: '2026-09-29T10:00:00Z',
  error: 'model timed out', output: 'Partial report', duration_ms: 8403, model: 'qwen', events: [{ type: 'message' } as never],
}
const STUBS = {
  Icon: true,
  MarkdownViewer: { props: ['content'], template: '<div class="md">{{ content }}</div>' },
  ExecutionAuditTrail: { template: '<div data-test="audit" />' },
}
const mounted: VueWrapper[] = []
function mountRun(props: Record<string, unknown> = {}) {
  const router = createAppRouter(createMemoryHistory())
  const w = mount(HistoricalRunDetails, { props: { run: RUN, ...props }, global: { plugins: [router], stubs: STUBS }, attachTo: document.body })
  mounted.push(w)
  return w
}

// Characterised on the old view first (plan D22), then updated: same content,
// confirmations through ConfirmDialog, D24 duration, optional close.
describe('HistoricalRunDetails', () => {
  afterEach(() => {
    mounted.splice(0).forEach((w) => w.unmount())
    document.body.innerHTML = ''
  })

  it('shows the outcome, duration, model, id, audit trail, error and report', () => {
    const w = mountRun()
    for (const text of ['nightly', 'Failed', '8.403s', 'qwen', 'model timed out', 'Partial report']) {
      expect(w.text()).toContain(text)
    }
    expect(w.find('button[aria-label="Copy ID run_0001abcdef"]').exists()).toBe(true)
    expect(w.find('[data-test="audit"]').exists()).toBe(true)
  })

  it('links to the automation and the workspace the run belongs to', () => {
    const w = mountRun()
    expect(decodeURIComponent(w.get('a[aria-label="Open automation nightly"]').attributes('href')!)).toBe('/automations/ws/nightly')
    expect(w.get('a[aria-label="Open workspace ws"]').attributes('href')).toBe('/workspaces/ws')
  })

  it('copies the final report and the final error', () => {
    const copies = mountRun().findAllComponents({ name: 'CopyButton' })
    expect(copies.map((c) => [c.props('title'), c.props('text')])).toEqual([
      ['Copy error', 'model timed out'],
      ['Copy report', 'Partial report'],
    ])
  })

  it('deletes the run, or all runs of the automation, only after confirming', async () => {
    const w = mountRun()
    await w.findAll('button').find((b) => b.text().includes('Delete run'))!.trigger('click')
    await flushPromises()
    expect(w.emitted('delete-run')).toBeUndefined()
    useConfirm().handleConfirm()
    await flushPromises()
    expect(w.emitted('delete-run')).toEqual([[RUN]])
    await w.findAll('button').find((b) => b.text().includes('Clear all runs'))!.trigger('click')
    await flushPromises()
    useConfirm().handleConfirm()
    await flushPromises()
    expect(w.emitted('delete-automation-runs')).toEqual([[{ name: 'nightly', workspace: 'ws' }]])
  })

  it('closes, unless embedded in a container that has its own close', async () => {
    const w = mountRun()
    await w.get('button[aria-label="Close run"]').trigger('click')
    expect(w.emitted('close')).toHaveLength(1)
    expect(mountRun({ embedded: true }).find('button[aria-label="Close run"]').exists()).toBe(false)
  })
})
