import { expect, test, type Page } from '@playwright/test'
import { expectNoSidewaysScroll, lookAt, mockAdminApi, setNetauditView } from './support/adminApi'

// Network → Timeline: one timeline. At both widths and in Simple and Advanced nothing may scroll sideways, every control is
// a touch target on a phone, a long unbroken name wraps and does not widen the page, and the quiet and failed states say
// what they are. The screenshots are for looking at.
const at = (hours: number) => new Date(Date.now() - hours * 3_600_000).toISOString()
const LONG = 'a-device-name-without-any-spaces-that-keeps-going-and-going-until-it-would-push-the-page-sideways-1234567890'
const BUSY = {
  rows: [
    { kind: 'related', group: 'alerts', at: at(1), mac: 'aa:bb:cc:00:00:09', ip: '192.168.1.77' },
    { kind: 'alert', group: 'alerts', at: at(1.2), alert_kind: 'decoy', text: 'A decoy service on this computer was touched by 192.168.1.77', count: 3 },
    { kind: 'new_device', group: 'devices', at: at(1.5), mac: 'aa:bb:cc:00:00:09', ip: '192.168.1.77', name: LONG },
    { kind: 'ports_changed', group: 'ports', at: at(5), day: true, opened: 2, closed: 1 },
    { kind: 'audit', group: 'findings', at: at(9), opened: 2, closed: 1 },
    { kind: 'address_changed', group: 'devices', at: at(30), mac: 'aa:bb:cc:00:00:03', ip: '192.168.1.9' },
  ],
}

// The timeline reads the audit's workspace from the automations list, so one audit has to exist for the explain button.
const AUDIT = { name: 'nightly', workspace: 'ws', profile: 'security-auditor', trigger_type: 'cron', trigger_value: '0 2 * * *', network_grant: 'lan', scan: { depth: 'thorough', checks: true }, history: [], is_running: false }

async function open(page: Page, view: 'simple' | 'advanced', activity: unknown, status = 200) {
  await setNetauditView(page, view)
  await mockAdminApi(page, { answers: { '/automations': [AUDIT] } })
  await page.route('**/admin/api/netaudit/activity*', (route) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(activity) }))
  await page.goto('network/timeline')
  await expect(page.getByRole('region', { name: 'What happened lately' })).toBeVisible()
}

for (const view of ['simple', 'advanced'] as const) {
  test(`activity, busy (${view}), fits the screen`, async ({ page }, testInfo) => {
    await open(page, view, BUSY)
    const card = page.locator('[data-test="activity"]')
    await expect(card.getByText('Possibly related')).toBeVisible()
    await expect(card.getByText('Today')).toBeVisible()
    await expect(card.getByText('Yesterday')).toBeVisible()
    await expectNoSidewaysScroll(page)
    if (view === 'advanced') await expect(card.getByText('aa:bb:cc:00:00:09 · 192.168.1.77').first()).toBeVisible()
    else await expect(card.getByText('aa:bb:cc:00:00:09')).toHaveCount(0)
    if (testInfo.project.name === 'mobile') {
      for (const target of [card.getByRole('radio').first(), card.getByRole('button', { name: 'Refresh' })]) {
        expect((await target.boundingBox())?.height).toBeGreaterThanOrEqual(44)
      }
    }
    await lookAt(card, testInfo, `activity-busy-${view}`)
  })

  test(`activity, quiet (${view}), says so`, async ({ page }, testInfo) => {
    await open(page, view, { rows: [] })
    await expect(page.getByText('Nothing has happened lately.')).toBeVisible()
    await expectNoSidewaysScroll(page)
    await lookAt(page.locator('[data-test="activity"]'), testInfo, `activity-quiet-${view}`)
  })

  test(`activity, a failed read (${view}), says it and offers a retry`, async ({ page }, testInfo) => {
    await open(page, view, { error: 'boom' }, 500)
    await expect(page.getByText('The activity could not be read')).toBeVisible()
    await expect(page.getByRole('button', { name: 'Retry' })).toBeVisible()
    await expectNoSidewaysScroll(page)
    await lookAt(page.locator('[data-test="activity"]'), testInfo, `activity-failed-${view}`)
  })
}

test('the Timeline tab is reachable from the tab strip', async ({ page }, testInfo) => {
  await setNetauditView(page, 'simple')
  await mockAdminApi(page, {})
  await page.goto('network')
  await page.getByRole('link', { name: 'Timeline' }).click()
  await expect(page).toHaveURL(/network\/timeline$/)
  await expectNoSidewaysScroll(page)
  if (testInfo.project.name === 'mobile') expect((await page.getByRole('link', { name: 'Timeline' }).boundingBox())?.height).toBeGreaterThanOrEqual(44)
})

for (const view of ['simple', 'advanced'] as const) {
  test(`explain this week (${view}): the answer and a refusal fit the screen`, async ({ page }, testInfo) => {
    await open(page, view, BUSY)
    let refuse = false
    await page.route('**/admin/api/netaudit/activity/summary*', (route) => refuse
      ? route.fulfill({ status: 409, contentType: 'application/json', body: JSON.stringify({ error: 'The model for this workspace runs in the cloud, and network data may only go to a model on this machine. Choose a local model for the workspace.' }) })
      : route.fulfill({ contentType: 'application/json', body: JSON.stringify({ summary: 'A new device appeared an hour ago, and a decoy service was touched soon after; they may be unrelated. Look at the Devices page first.', model: 'a-local-model-with-a-long-name-qwen3-30b-a3b-instruct' }) }))
    const card = page.locator('[data-test="activity"]')
    const button = card.getByRole('button', { name: 'Explain this week' })
    await expect(button).toBeVisible()
    if (testInfo.project.name === 'mobile') expect((await button.boundingBox())?.height).toBeGreaterThanOrEqual(44)
    await button.click()
    await expect(card.locator('[data-test="explain-result"]')).toContainText('Look at the Devices page first.')
    await expectNoSidewaysScroll(page)
    await lookAt(card, testInfo, `timeline-explain-${view}`)
    refuse = true
    await card.getByRole('button', { name: 'Explain this week' }).click()
    await expect(card.locator('[data-test="explain-error"]')).toContainText('on this machine')
    await expectNoSidewaysScroll(page)
    await lookAt(card, testInfo, `timeline-explain-refused-${view}`)
  })
}
