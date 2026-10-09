import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { AutomationRun } from '../../../../types/dispatcher'

const confirm = vi.fn()
vi.mock('../../../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm }) }))

import WorkspaceActivity from '../../../../components/AgentIde/workspace/WorkspaceActivity.vue'

// Characterised before its restyle (plan D22), then moved to the rebuilt list:
// newest first, outcome and duration shown, delete asks first (now through
// ConfirmDialog instead of an inline confirm).
const run = (id: string, minute: number, over: Partial<AutomationRun> = {}): AutomationRun =>
  ({ id: `run_${id}`, workspace_id: 'ws', automation_name: `auto-${id}`, timestamp: `2026-09-29T10:0${minute}:00Z`, error: '', output: '', duration_ms: 125000, model: '', ...over })
const HISTORY = [run('old', 1), run('new', 5, { error: 'boom' })]

const mountActivity = (history = HISTORY) => mount(WorkspaceActivity, { props: { history }, global: { stubs: { Icon: true } } })
const openButtons = (w: ReturnType<typeof mountActivity>) => w.findAll('button').filter((b) => /^open run/i.test(b.attributes('aria-label') ?? ''))

describe('WorkspaceActivity', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    confirm.mockResolvedValue(true)
  })

  it('lists runs newest first with outcome, model and duration', () => {
    const w = mountActivity()
    expect(openButtons(w).map((b) => b.attributes('aria-label'))).toEqual(['Open run auto-new', 'Open run auto-old'])
    const items = w.findAll('li')
    expect(items[0]!.text()).toContain('Failed')
    expect(items[1]!.text()).toContain('Completed')
    expect(w.text()).toContain('Default')
    expect(w.text()).toContain('2m 5s')
  })

  it('opens a run, and deletes one only after confirming', async () => {
    const w = mountActivity()
    await openButtons(w)[0]!.trigger('click')
    expect(w.emitted('select-run')).toEqual([[HISTORY[1]]])

    const del = w.get('button[aria-label="Delete run auto-new"]')
    confirm.mockResolvedValueOnce(false)
    await del.trigger('click')
    await flushPromises()
    expect(w.emitted('delete-run')).toBeUndefined()
    await del.trigger('click')
    await flushPromises()
    expect(confirm).toHaveBeenLastCalledWith(expect.objectContaining({ type: 'warning' }))
    expect(w.emitted('delete-run')).toEqual([[HISTORY[1]]])
  })

  it('says when there is no history', () => {
    expect(mountActivity([]).text()).toContain('No runs yet')
  })
})
