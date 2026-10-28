import { expect, test, type Page } from '@playwright/test'
import { expectNoSidewaysScroll, lookAt, mockAdminApi, setNetauditView } from './support/adminApi'

// The audit's edit form with its scan settings, Simple and Advanced at both widths: nothing may scroll sideways, and the
// screenshots are for looking at.
const automation = {
  id: 'ws/network-audit', workspace: 'ws', name: 'network-audit', task_file: 'network-audit.md', strategy: 'persistent', trigger: 'cron',
  trigger_value: '0 2 * * *', profile: 'security-auditor', network_grant: 'lan', journal: false,
  scan: { depth: 'thorough', checks: true }, history: [],
}
const profiles = [{ id: 'security-auditor', name: 'Security auditor', summary: 'Scans the networks you allowed.', source: 'builtin', answer_first: false, may_ask: false, memory_save: 'off', reminders: false, default_for: [] }]

async function open(page: Page, view: 'simple' | 'advanced') {
  await setNetauditView(page, view)
  await mockAdminApi(page, { answers: { '/netaudit/readiness': { ready: true, requirements: [] }, '/automations': [automation], '/workspaces': [{ id: 'ws' }], '/profiles': profiles } })
  await page.goto('automations/ws%2Fnetwork-audit/edit')
  await expect(page.getByRole('heading', { name: 'What the audit scans' })).toBeVisible()
}

for (const view of ['simple', 'advanced'] as const) {
  test(`audit form scan settings (${view}) fit the screen`, async ({ page }, testInfo) => {
    await open(page, view)
    await expectNoSidewaysScroll(page)
    await page.getByRole('heading', { name: 'What the audit scans' }).scrollIntoViewIfNeeded()
    await lookAt(page, testInfo, `form-${view}.png`, true)
  })
}
