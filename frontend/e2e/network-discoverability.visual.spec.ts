import { expect, test } from '@playwright/test'
import { expectNoSidewaysScroll, mockAdminApi } from './support/adminApi'

// What each switch does, where a person can find it: a fold on every module tile, one coverage line on the Network overview
// that says what is off and what each would watch, and the Help's list. At both widths nothing scrolls sideways, and on a phone the
// folds and the button are 44 px targets. The data is invented; the shape is the server's.
const about = (id: string) => ({ watches: `${id} watches one thing.`, never: `${id} never does the other.`, shows_up: `${id} shows up in the findings.`, settings: true })
const module = (id: string, title: string, over: Record<string, unknown> = {}) => ({
  id, title, purpose: `${title} does its one job.`, about: about(id), finds: [], sends: 'nothing', step_up: false, enabled: true, needs: [], ...over,
})
const seen = new Date(Date.now() - 5 * 60_000).toISOString()
const OVERVIEW = {
  posture: 'quiet', devices: { known: 12, unnamed: 3, new: 0, active: 9 }, findings: { scanned: true, scanned_at: seen, open: 0, serious: 0, new: 0 },
  gateway_changed: false, shared_address: false, presence: { last_check: seen }, series: [], recent: [], attack_paths_on: false,
}
const NETWORK = {
  id: 'network', title: 'Network', purpose: '', actions: [], summary: { on: 1, total: 3, stopped: 1, blocked: 0 },
  layers: [
    { id: 'listen', title: 'What the network says', purpose: 'What the router and the devices tell you by themselves', modules: [
      module('phone_home', 'Phone-home watch', { health: 'stopped' }),
      module('login_watch', 'Failed logins, this computer', { enabled: false, finds: ['host_login_failures'] }),
    ] },
    { id: 'discover', title: 'Who is on the network', modules: [module('presence', 'Presence check', { health: 'working' })] },
  ],
}

test('a module tile folds what it does, and the fold is a touch target on a phone', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: { '/stacks/network': NETWORK } })
  await page.goto('network/modules')
  const tile = page.locator('[data-test="module-login_watch"]')
  const fold = tile.locator('[data-test="module-about"] button')
  await expect(fold).toHaveAttribute('aria-expanded', 'false')
  await fold.click()
  await expect(tile).toContainText('login_watch watches one thing.')
  await expect(tile).toContainText('login_watch never does the other.')
  await expect(tile.locator('[data-test="module-settings"]')).toBeVisible()
  await expect(page.getByRole('heading', { name: '02 — What the network says' })).toBeVisible()
  await expectNoSidewaysScroll(page)
  if (testInfo.project.name === 'mobile') expect((await fold.boundingBox())?.height).toBeGreaterThanOrEqual(44)
})

test('the overview says how many protections are watching and what each one that is off would watch', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: { '/stacks/network': NETWORK, '/netaudit/overview': OVERVIEW } })
  await page.goto('network')
  const line = page.locator('[data-test="coverage-line"]')
  await expect(line).toContainText('2 of 3 protections are watching · 1 stopped')
  await line.locator('button').first().click()
  await expect(page.locator('[data-test="coverage-off"]')).toContainText('Failed logins, this computer: login_watch watches one thing.')
  await expect(page.locator('[data-test="coverage-stopped"]')).toContainText('Phone-home watch has stopped')
  await expectNoSidewaysScroll(page)
  if (testInfo.project.name === 'mobile') {
    expect((await page.getByRole('button', { name: 'Open the Modules tab' }).boundingBox())?.height).toBeGreaterThanOrEqual(44)
  }
})
