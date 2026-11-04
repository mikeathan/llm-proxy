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
    await expect(card.getByText('Possibly related', { exact: true })).toBeVisible()
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

// A week of real-looking activity: the page must read as a short list, not a wall.
const day = (d: number, h: number) => new Date(new Date(Date.now() - d * 86_400_000).setHours(h, 0, 0, 0)).toISOString()
const WEEK = {
  rows: [
    { kind: 'ports_changed', group: 'ports', at: day(0, 12), day: true, opened: 1, closed: 0 },
    { kind: 'audit', group: 'findings', at: day(0, 2), opened: 2, closed: 2 },
    { kind: 'ports_changed', group: 'ports', at: day(2, 23), day: true, opened: 0, closed: 1 },
    { kind: 'ports_changed', group: 'ports', at: day(3, 23), day: true, opened: 1, closed: 1 },
    { kind: 'alert', group: 'alerts', at: day(3, 14), alert_kind: 'presence', text: 'Network audit: 1 new device: 192.168.50.161 (private address). Open the network audit to review.', count: 1 },
    { kind: 'new_device', group: 'devices', at: day(3, 13), mac: '5a:72:d3:96:a8:2f', ip: '192.168.50.161', name: 'Lenovo-Tab-M11' },
    { kind: 'alert', group: 'alerts', at: day(3, 9), alert_kind: 'decoy', text: 'Network audit: 1 device connected to a decoy service (a fake service nothing legitimate uses): 192.168.50.127. Open the network audit to review.', count: 1 },
    { kind: 'new_device', group: 'devices', at: day(5, 11), mac: 'f0:24:f9:ee:81:1c', ip: '192.168.50.160', name: 'eveything presence lite' },
    { kind: 'new_device', group: 'devices', at: day(5, 10), mac: '66:75:cb:6a:b9:d6', ip: '192.168.50.64', name: 'iPhone' },
    { kind: 'new_device', group: 'devices', at: day(5, 9), mac: '5a:b5:3a:c5:d7:e5', ip: '192.168.50.235' },
    { kind: 'new_device', group: 'devices', at: day(5, 8), mac: 'be:b4:ca:64:66:8f', ip: '192.168.50.124', name: 'Redmi-Pad-2' },
  ],
}

for (const view of ['simple', 'advanced'] as const) {
  test(`a week of activity (${view}) reads as a short list`, async ({ page }, testInfo) => {
    await open(page, view, WEEK)
    const card = page.locator('[data-test="activity"]')
    const days = card.locator('[data-test="activity-day"]')
    await expect(days.first()).toBeVisible()
    // The latest two days are open; the older ones are one line of counts each.
    expect(await days.locator('ul').count()).toBe(2)
    await expect(card.locator('[data-test="day-summary"]').last()).toContainText('4 new devices')
    if (view === 'simple') await expect(card.getByText('Network audit: 1 new device')).toHaveCount(0)
    await expectNoSidewaysScroll(page)
    if (testInfo.project.name === 'mobile') expect((await days.first().getByRole('button').boundingBox())?.height).toBeGreaterThanOrEqual(44)
    await lookAt(card, testInfo, `timeline-week-${view}`)
    await card.locator('[data-test="day-toggle"]').last().click()
    await expect(card.locator('[data-test="day-toggle"]').last()).toHaveAttribute('aria-expanded', 'true')
    await expectNoSidewaysScroll(page)
    await lookAt(card, testInfo, `timeline-week-opened-${view}`)
  })
}

const iso = (d: number) => {
  const t = new Date(Date.now() - d * 86_400_000)
  return `${t.getFullYear()}-${String(t.getMonth() + 1).padStart(2, '0')}-${String(t.getDate()).padStart(2, '0')}`
}
const ROUTER = {
  rows: [
    { kind: 'router_blocks', group: 'router', at: day(0, 12), day: true, count: 1204, ports: [{ port: 22, count: 700 }, { port: 3306, count: 300 }, { port: 8123, count: 100 }] },
    { kind: 'router_blocks', group: 'router', at: day(1, 23), day: true, count: 2310, ports: [{ port: 23, count: 1500 }, { port: 22, count: 400 }] },
    { kind: 'new_device', group: 'devices', at: day(1, 9), mac: '5a:72:d3:96:a8:2f', ip: '192.168.50.161', name: 'Lenovo-Tab-M11' },
  ],
  router: {
    today: 1204,
    today_ports: [{ port: 22, count: 700 }, { port: 3306, count: 300 }, { port: 8123, count: 100 }],
    days: Array.from({ length: 14 }, (_, i) => ({ date: iso(13 - i), total: 400 + ((i * 337) % 1900) })),
  },
}

for (const view of ['simple', 'advanced'] as const) {
  test(`the router card and filter (${view}) fit the screen`, async ({ page }, testInfo) => {
    await open(page, view, ROUTER)
    const card = page.locator('[data-test="activity"]')
    const router = card.locator('[data-test="router-card"]')
    await expect(router).toContainText('1,204')
    await expect(router).toContainText('SSH (22) 700')
    await expectNoSidewaysScroll(page)
    await expect(router.getByText('cannot check')).toBeHidden()
    await router.getByText('What is this number?').click()
    await expect(router.getByText('cannot check')).toBeVisible()
    await expectNoSidewaysScroll(page)
    const chip = card.getByRole('radio', { name: 'Router' })
    await expect(chip).toBeVisible()
    if (testInfo.project.name === 'mobile') expect((await chip.boundingBox())?.height).toBeGreaterThanOrEqual(44)
    await lookAt(card, testInfo, `timeline-router-${view}`)
    await chip.click()
    await expect(card.locator('li')).toHaveCount(2)
    await expectNoSidewaysScroll(page)
  })
}

test('the Overview shows the router figure only when the router has been counted', async ({ page }, testInfo) => {
  await setNetauditView(page, 'simple')
  const overview = (withBlocks: boolean) => ({
    posture: 'quiet', devices: { known: 9, unnamed: 1, new: 0, active: 8 }, findings: { scanned: true, scanned_at: new Date().toISOString(), open: 3, serious: 0, new: 0 },
    gateway_changed: false, shared_address: false, presence: { last_check: new Date().toISOString() }, recent: [],
    series: [iso(1), iso(0)].map((date, i) => ({ date, devices_up: 8, devices: 9, unnamed: 1, findings: 3, serious: 0, audited: true, ...(withBlocks ? { blocked: i ? 1204 : 2310 } : {}) })),
  })
  await mockAdminApi(page, { answers: { '/automations': [AUDIT], '/netaudit/overview': overview(true) } })
  await page.goto('network')
  await expect(page.locator('[data-test="stats"]').getByText('Refused at the router')).toBeVisible()
  await expectNoSidewaysScroll(page)
  await lookAt(page.locator('[data-test="stats"]'), testInfo, 'overview-router-stat')
  await mockAdminApi(page, { answers: { '/automations': [AUDIT], '/netaudit/overview': overview(false) } })
  await page.goto('network')
  await expect(page.locator('[data-test="stats"]')).toBeVisible()
  await expect(page.locator('[data-test="stats"]').getByText('Refused at the router')).toHaveCount(0)
})

