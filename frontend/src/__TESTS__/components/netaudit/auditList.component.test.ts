import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const dispatcher = vi.hoisted(() => ({ listAutomations: vi.fn(), listWorkspaces: vi.fn() }))
vi.mock('../../../services/automation/dispatcherService', () => ({ DispatcherService: dispatcher }))
vi.mock('vue-router', async (importOriginal) => ({ ...(await importOriginal<typeof import('vue-router')>()), useRouter: () => ({ push: vi.fn() }) }))

import AuditList from '../../../components/netaudit/AuditList.vue'
import { useNetauditViewMode } from '../../../composables/netaudit/useNetauditViewMode'

const run = (id: string, over: Record<string, unknown> = {}) => ({ id, automation_name: 'audit', timestamp: '2026-10-29T02:00:00Z', output: '', error: '', duration_ms: 134_000, model: 'm', ...over })
const audit = (history: unknown[]) => ({
  id: 'ws/audit', workspace: 'ws', name: 'audit', task_file: 'audit.md', strategy: 'persistent', trigger: 'cron', trigger_value: '0 2 * * *', profile: 'security-auditor', history,
})

describe('AuditList run history', () => {
  beforeEach(() => {
    dispatcher.listAutomations.mockReset()
    dispatcher.listWorkspaces.mockReset().mockResolvedValue([])
    useNetauditViewMode().set('simple')
  })

  it('keeps the list to the audit, its schedule and its last run in Simple', async () => {
    dispatcher.listAutomations.mockResolvedValue([audit([run('r1'), run('r2')])])
    const w = mount(AuditList)
    await flushPromises()
    expect(w.get('[data-test="audits"]').text()).toContain('Last run')
    expect(w.find('[data-test="audit-history"]').exists()).toBe(false)
  })

  it('adds the recent runs with how they ended and how long they took in Advanced, newest first and at most five', async () => {
    useNetauditViewMode().set('advanced')
    const history = [run('r1'), run('r2', { error: 'scan failed', duration_ms: 3_000 }), run('r3'), run('r4'), run('r5'), run('r6')]
    dispatcher.listAutomations.mockResolvedValue([audit(history)])
    const w = mount(AuditList)
    await flushPromises()
    const rows = w.findAll('[data-test="audit-run"]')
    expect(rows).toHaveLength(5)
    expect(rows[0]!.text()).toContain('Completed')
    expect(rows[0]!.text()).toContain('2m 14s')
    expect(rows[1]!.text()).toContain('Failed')
    expect(rows[1]!.text()).toContain('3.000s')
    // Everything Simple shows is still there.
    expect(w.get('[data-test="audits"]').text()).toContain('Last run')
    expect(w.findAll('button').some((b) => b.text() === 'Open')).toBe(true)
  })
})
