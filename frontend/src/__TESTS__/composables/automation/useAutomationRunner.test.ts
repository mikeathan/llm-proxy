import { afterEach, beforeEach, describe, it, expect, vi } from 'vitest'
import { effectScope, ref } from 'vue'
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
    expect(r.lastTriggerResult.value).toBe('Started a')
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

// The server answers a trigger as soon as the run is admitted, but only marks the automation running a moment later
// (after the workspace lock and the pre-flight). Until then the button must not look as if the click did nothing.
describe('useAutomationRunner: starting', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  const idle = (): Automation => ({ id: 'ws/a', workspace: 'ws', name: 'a', is_running: false, history: [] }) as unknown as Automation

  function start(status: 'started' | 'queued' | 'skipped' = 'started') {
    const automations = ref([idle()])
    const trigger = vi.fn().mockResolvedValue({ status })
    const fetchAutomations = vi.fn().mockResolvedValue(undefined)
    const scope = effectScope()
    const r = scope.run(() => useAutomationRunner(automations, trigger, vi.fn(), vi.fn().mockResolvedValue(undefined), fetchAutomations))!
    r.selectAutomation(automations.value[0]!)
    return { r, automations, fetchAutomations, scope }
  }
  const mutate = (automations: ReturnType<typeof start>['automations'], over: Partial<Automation>) => { automations.value = [{ ...automations.value[0]!, ...over }] }

  it('stays "starting" after the trigger answers, and follows the server until it says running', async () => {
    const { r, automations, fetchAutomations, scope } = start()
    await r.handleTrigger()
    expect(r.triggering.value).toBe(false)
    expect(r.starting.value).toBe(true)
    fetchAutomations.mockClear()
    await vi.advanceTimersByTimeAsync(3000)
    expect(fetchAutomations).toHaveBeenCalledTimes(3)
    expect(r.starting.value).toBe(true)
    mutate(automations, { is_running: true })
    await vi.advanceTimersByTimeAsync(1000)
    expect(r.starting.value).toBe(false)
    fetchAutomations.mockClear()
    await vi.advanceTimersByTimeAsync(5000)
    expect(fetchAutomations).not.toHaveBeenCalled()
    scope.stop()
  })

  it('is not starting when the run is already running by the time the trigger returns', async () => {
    const { r, automations, scope } = start()
    mutate(automations, { is_running: true })
    await r.handleTrigger()
    expect(r.starting.value).toBe(false)
    scope.stop()
  })

  it.each(['queued', 'skipped'] as const)('is not starting when the trigger was %s: nothing is about to run', async (status) => {
    const { r, scope } = start(status)
    await r.handleTrigger()
    expect(r.starting.value).toBe(false)
    scope.stop()
  })

  it('stops waiting when the run ended without ever showing as running (it failed to start)', async () => {
    const { r, automations, scope } = start()
    await r.handleTrigger()
    mutate(automations, { history: [{ id: 'r1' } as AutomationRun] })
    await vi.advanceTimersByTimeAsync(1000)
    expect(r.starting.value).toBe(false)
    scope.stop()
  })

  it('gives up after a minute rather than waiting forever', async () => {
    const { r, scope } = start()
    await r.handleTrigger()
    await vi.advanceTimersByTimeAsync(61_000)
    expect(r.starting.value).toBe(false)
    scope.stop()
  })

  it('stops polling when the page goes away', async () => {
    const { r, fetchAutomations, scope } = start()
    await r.handleTrigger()
    scope.stop()
    fetchAutomations.mockClear()
    await vi.advanceTimersByTimeAsync(5000)
    expect(fetchAutomations).not.toHaveBeenCalled()
  })
})
