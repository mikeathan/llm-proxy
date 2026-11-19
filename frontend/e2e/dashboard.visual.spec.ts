import { expect, test } from '@playwright/test'
import { expectNoSidewaysScroll, lookAt, mockAdminApi } from './support/adminApi'

// The dashboard: boards of cards. Home ships minimal (nothing a stack board repeats; the person adds more), a stack ships its own board, and a board re-flows from its
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

test('Home ships minimal: one card no stack board repeats', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: { '/netaudit/overview': OVERVIEW, '/metrics': METRICS, '/dashboard': { boards: [] } } })
  await page.goto('dashboard')
  await expect(page.getByRole('heading', { name: 'Dashboard', level: 1 })).toBeVisible()
  await expect(page.locator('[data-test="board-tab"][aria-current="page"]')).toHaveText('Home')
  await expect(page.locator('[data-test="board-tab"]')).toHaveText(['Home', 'LLM and host', 'Network', 'Radio', 'Automations'])
  await expect(page.locator('[data-test="card-assistant-recent"]')).toBeVisible()
  await expect(page.locator('[data-test="dashboard-grid"] > div')).toHaveCount(1)
  await expectNoSidewaysScroll(page)
  await lookAt(page, testInfo, 'dashboard-home', true)
  if (testInfo.project.name === 'mobile') {
    const grid = await page.locator('[data-test="dashboard-grid"]').boundingBox()
    for (const card of await page.locator('[data-test="dashboard-grid"] > div').all()) expect((await card.boundingBox())?.width).toBeGreaterThan((grid?.width ?? 0) - 2)
    for (const tab of await page.locator('[data-test="board-tab"]').all()) expect((await tab.boundingBox())?.height).toBeGreaterThanOrEqual(44)
  }
})

test('the Network board holds the overview, the devices and the findings', async ({ page }, testInfo) => {
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
  await page.goto('dashboard?board=host')
  await expect(page.locator('[data-test="host-health-card"]')).toBeVisible() // the cards load as they are first shown
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

// Real layout, so happy-dom cannot stand in: resizing one card must not change its neighbours, and a short card leaves no hole under it.
test('arranging on a wide screen: resizing never stretches the neighbour, and "later" moves a card', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'desktop', 'the grid is one column on a phone')
  await mockAdminApi(page, { answers: { '/netaudit/overview': OVERVIEW, '/metrics': METRICS, '/dashboard': { boards: [] } } })
  await page.goto('dashboard?board=network')
  await expect(page.locator('[data-test="network-posture-card"]')).toBeVisible()
  await page.locator('[data-test="arrange"]').click()
  const card = (id: string) => page.locator(`[data-test="card-${id}"]`)
  const box = async (id: string) => (await card(id).boundingBox())!
  // The first row holds Network posture (8) and What changed (4).
  const before = await box('network-changes')
  const own = card('network-posture')
  await own.locator('[aria-label="Height of Network posture"] button', { hasText: '2×' }).click()
  await expect.poll(async () => (await box('network-posture')).height).toBeGreaterThan(400)
  expect((await box('network-changes')).height).toBe(before.height)
  await own.locator('[aria-label="Height of Network posture"] button', { hasText: '1×' }).click()
  await own.locator('[aria-label="Width of Network posture"] button', { hasText: 'S' }).click()
  await expect.poll(async () => (await box('network-posture')).width).toBeLessThan(before.width)
  expect((await box('network-changes')).height).toBe(before.height)
  // "Later" changes where the card sits: it goes after the card that followed it.
  const at = await box('network-posture')
  await own.locator('[data-move="later"]').click()
  await expect.poll(async () => JSON.stringify(await box('network-posture'))).not.toBe(JSON.stringify(at))
})

// A tall card must not leave a hole under the short cards beside it: the next card goes under the shorter one, not below the whole row.
test('a tall card leaves no hole: the next card is placed under a short one', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'desktop', 'the grid is one column on a phone')
  const layout = { boards: [{ id: 'home', name: 'Home', cards: [
    { id: 'reminders', cols: 4, rows: 1 }, { id: 'host-health', cols: 4, rows: 1 }, { id: 'radio-around', cols: 4, rows: 2 }, { id: 'running-now', cols: 4, rows: 1 }, { id: 'recent-runs', cols: 4, rows: 1 },
  ] }] }
  await mockAdminApi(page, { answers: { '/netaudit/overview': OVERVIEW, '/metrics': METRICS, '/dashboard': layout } })
  await page.goto('dashboard')
  await expect(page.locator('[data-test="card-radio-around"]')).toBeVisible()
  await expect(page.locator('[data-test="card-running-now"]')).toBeVisible()
  const box = async (id: string) => (await page.locator(`[data-test="card-${id}"]`).boundingBox())!
  // The cards load as they are first shown and change height as they do, so the packing is read until it settles.
  await expect.poll(async () => {
    const [radar, next, ...above] = await Promise.all(['radio-around', 'running-now', 'reminders', 'host-health'].map(box))
    const shortest = Math.min(...above.map((b) => b.y + b.height))
    return radar.height > 400 && next.y < radar.y + radar.height - 100 && next.x < radar.x && next.y < shortest + 40
  }).toBe(true) // beside the tall card, not below it; under a short card; a gap and no more
})

test('a card a stack page links to is offered when the saved board predates it', async ({ page }) => {
  const saved = { boards: [{ id: 'home', name: 'Home', cards: [{ id: 'host-health', cols: 12, rows: 1 }] }, { id: 'network', name: 'Network', cards: [{ id: 'network-posture', cols: 8, rows: 1 }] }] }
  await mockAdminApi(page, { answers: { '/netaudit/overview': OVERVIEW, '/metrics': METRICS, '/dashboard': saved } })
  await page.goto('network/traffic') // an old tab address: it opens the card on the board
  await expect(page.locator('[data-test="add-wanted-card"]')).toBeVisible()
  await page.locator('[data-test="add-wanted-card"]').click()
  await expect(page.locator('[data-test="card-network-traffic"]')).toBeVisible()
  await expect(page.locator('[data-test="add-wanted-card"]')).toHaveCount(0)
})

test('the stack pages link to their own board', async ({ page }) => {
  await mockAdminApi(page)
  await page.goto('network/modules')
  await page.locator('[data-test="open-board"]').click()
  await expect(page.locator('[data-test="board-tab"][aria-current="page"]')).toHaveText('Network')
})
