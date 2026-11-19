import { expect, test } from '@playwright/test'
import { expectNoSidewaysScroll, lookAt, mockAdminApi } from './support/adminApi'

// The dashboard: boards of cards. Home mixes cards from every part of the system, a stack ships its own board, and a board re-flows from its
// layout menu. On a phone every card is one full-width block, one after another. The screenshots are for looking at.
const seen = new Date(Date.now() - 5 * 60_000).toISOString()
const day = (offset: number, up: number, devices: number, findings: number) => ({
  date: new Date(Date.now() - offset * 86_400_000).toISOString().slice(0, 10), devices_up: up, devices, unnamed: 3, findings, serious: findings ? 1 : 0, audited: true,
})
const OVERVIEW = {
  posture: 'attention', devices: { known: 12, unnamed: 3, new: 2, active: 9 }, findings: { scanned: true, scanned_at: seen, open: 4, serious: 1, new: 1 },
  gateway_changed: true, shared_address: false, presence: { last_check: seen },
  series: [day(4, 7, 10, 6), day(3, 8, 11, 5), day(2, 8, 11, 5), day(1, 9, 12, 4), day(0, 9, 12, 4)],
  recent: [{ kind: 'address_changed', at: seen, ip: '192.168.1.9' }, { kind: 'new_device', at: seen, ip: '192.168.1.77' }],
}
const METRICS = { llm_tokens_per_sec: 42.1, load_percent: 31, mem_used_mb: 9_400, mem_total_mb: 32_000, gpu_error: 'No GPU detected' }

test('Home: cards from every part of the system, one board of them', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: { '/netaudit/overview': OVERVIEW, '/metrics': METRICS, '/dashboard': { boards: [] } } })
  await page.goto('dashboard')
  await expect(page.getByRole('heading', { name: 'Dashboard', level: 1 })).toBeVisible()
  await expect(page.locator('[data-test="board-tab"][aria-current="page"]')).toHaveText('Home')
  await expect(page.locator('[data-test="board-tab"]')).toHaveText(['Home', 'LLM and host', 'Network', 'Radio', 'Automations'])
  await expect(page.locator('[data-test="host-health-card"]')).toBeVisible()
  await expect(page.locator('[data-test="network-posture-card"]')).toContainText('Needs a look')
  await expectNoSidewaysScroll(page)
  await lookAt(page, testInfo, 'dashboard-home', true)
  if (testInfo.project.name === 'mobile') {
    const grid = await page.locator('[data-test="dashboard-grid"]').boundingBox()
    for (const card of await page.locator('[data-test="dashboard-grid"] > div').all()) expect((await card.boundingBox())?.width).toBeGreaterThan((grid?.width ?? 0) - 2)
    for (const tab of await page.locator('[data-test="board-tab"]').all()) expect((await tab.boundingBox())?.height).toBeGreaterThanOrEqual(44)
  }
})

test('the Network board holds the overview the Network page used to open on', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: { '/netaudit/overview': OVERVIEW, '/metrics': METRICS, '/dashboard': { boards: [] } } })
  await page.goto('dashboard?board=network')
  await expect(page.locator('[data-test="board-tab"][aria-current="page"]')).toHaveText('Network')
  await expect(page.locator('[data-test="network-posture-card"]')).toContainText("The router's hardware address changed")
  await expect(page.locator('[data-test="network-figures-card"] [data-test="stat-value"]').first()).toHaveText('12')
  await expect(page.locator('[data-test="network-changes-card"]')).toContainText('192.168.1.77 appeared on the network')
  await expectNoSidewaysScroll(page)
  await lookAt(page, testInfo, 'dashboard-network', true)
})

test('arranging: a layout re-flows the board, a card is resized, and the board offers to be put back', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: { '/netaudit/overview': OVERVIEW, '/metrics': METRICS, '/dashboard': { boards: [] } } })
  await page.goto('dashboard')
  await expect(page.locator('[data-test="network-posture-card"]')).toBeVisible() // the cards load as they are first shown
  await page.locator('[data-test="layout-select"]').selectOption('two-columns')
  await expect(page.locator('[data-test="reset-board"]')).toBeVisible()
  if (testInfo.project.name === 'desktop') {
    const [a, b] = await page.locator('[data-test="dashboard-grid"] > div').all()
    expect((await a!.boundingBox())?.y).toBe((await b!.boundingBox())?.y) // two to a row
  }
  await page.locator('[data-test="arrange"]').click()
  await expect(page.locator('[data-test="card-controls"]').first()).toBeVisible()
  await expect(page.locator('[data-test="card-remove"]').first()).toBeVisible()
  await lookAt(page, testInfo, 'dashboard-arrange', true)
  await expectNoSidewaysScroll(page)
  await page.locator('[data-test="reset-board"]').click()
  await expect(page.locator('[data-test="reset-board"]')).toHaveCount(0)
})

test('the stack pages link to their own board', async ({ page }) => {
  await mockAdminApi(page)
  await page.goto('network')
  await page.locator('[data-test="open-board"]').click()
  await expect(page.locator('[data-test="board-tab"][aria-current="page"]')).toHaveText('Network')
})
