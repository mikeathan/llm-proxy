import { describe, it, expect, vi } from 'vitest'
import { ref } from 'vue'
import { useAutomationRunner } from '../../../composables/automation/useAutomationRunner'
import type { Automation, AutomationRun } from '../../../types/dispatcher'

const A = { id: 'ws/a', workspace: 'ws', name: 'a', is_running: false } as Automation
const B = { id: 'ws/b', workspace: 'ws', name: 'b', is_running: true } as Automation

function setup(trigger = vi.fn().mockResolvedValue(undefined)) {
  const deps = { trigger, stop: vi.fn().mockResolvedValue(undefined), refreshHistory: vi.fn().mockResolvedValue(undefined), fetchAutomations: vi.fn().mockResolvedValue(undefined) }
  const r = useAutomationRunner(ref([A, B]), deps.trigger, deps.stop, deps.refreshHistory, deps.fetchAutomations)
  return { r, deps }
}

// Characterisation (plan D22), before the Automations pages use it per route.
describe('useAutomationRunner', () => {
  it('selects an automation and knows whether its workspace is busy', () => {
    const { r } = setup()
    r.selectAutomation(A)
    expect(r.selectedAutomation.value).toEqual(A)
    expect(r.anyRunningInSelectedWorkspace.value).toBe(true)
  })

  it('selects a run together with its automation', () => {
    const { r } = setup()
    const run = { id: 'r1', automation_name: 'a', workspace_id: 'ws' } as AutomationRun
    r.selectRun(run)
    expect(r.selectedAutomationId.value).toBe('ws/a')
    expect(r.selectedRun.value).toEqual(run)
    expect(r.findAutomationForRun({ ...run, automation_name: 'gone' })).toBeUndefined()
  })

  it('triggers the selected automation, reports the outcome and refreshes', async () => {
    const { r, deps } = setup()
    r.selectAutomation(A)
    await r.handleTrigger()
    expect(deps.trigger).toHaveBeenCalledWith('ws', 'a')
    expect(r.lastTriggerResult.value).toBe('Triggered a successfully')
    expect(r.triggering.value).toBe(false)
    expect(deps.fetchAutomations).toHaveBeenCalled()
    expect(deps.refreshHistory).toHaveBeenCalled()
  })

  it('reports a failed trigger', async () => {
    const { r } = setup(vi.fn().mockImplementation(async () => { throw new Error('busy') }))
    r.selectAutomation(A)
    await r.handleTrigger()
    expect(r.lastTriggerResult.value).toBe('Failed to trigger a')
  })

  it('replays a recording and stops a workspace', async () => {
    const { r, deps } = setup()
    await r.handleReplayRecording(A, { id: 'rec1' } as never)
    expect(deps.trigger).toHaveBeenCalledWith('ws', 'a', 'rec1')
    r.selectAutomation(B)
    await r.handleStop()
    expect(deps.stop).toHaveBeenCalledWith('ws')
    expect(r.lastTriggerResult.value).toBe('Stopped b')
  })

  it('clears the selection', () => {
    const { r } = setup()
    r.selectAutomation(A)
    r.clearSelection()
    expect(r.selectedAutomation.value).toBeNull()
  })
})
