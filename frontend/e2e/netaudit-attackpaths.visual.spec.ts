import { expect, test } from '@playwright/test'
import { expectNoSidewaysScroll, lookAt, mockAdminApi } from './support/adminApi'

// Network → Overview → Ways in that matter: each path reads top to bottom (where it starts, the weaknesses on it, what it ends at,
// the one fix), nothing scrolls sideways at either width, and a path of accepted findings is kept but quieter.
const seen = new Date(Date.now() - 5 * 60_000).toISOString()
const base = {
  posture: 'attention', devices: { known: 12, unnamed: 3, new: 0, active: 9 }, findings: { scanned: true, scanned_at: seen, open: 4, serious: 1, new: 0 },
  gateway_changed: false, shared_address: false, presence: { last_check: seen }, series: [], recent: [], attack_paths_on: true,
}
const ssh = { key: 'this-computer|tcp|0|host_ssh_password_login', check: 'host_ssh_password_login', title: 'SSH accepts password login', severity: 'low' }
const admin = { key: '192.168.1.1|tcp|80|admin_service_exposed', check: 'admin_service_exposed', title: 'Admin or metrics service open to the network', port: 80, severity: 'medium' }
const telnet = { key: '192.168.1.30|tcp|23|telnet_open', check: 'telnet_open', title: 'Telnet is open', port: 23, severity: 'high', accepted: true }
const PATHS = [
  { id: 'internet|192.168.1.1', entry: { kind: 'internet', text: 'The internet can reach port 1194 on your router' }, steps: [admin], prize: { kind: 'router', where: '192.168.1.1' }, fix: { key: admin.key, text: 'Turn on the service’s own login, or make it listen on localhost only.' }, breaks: 2 },
  { id: 'network|192.168.1.1', entry: { kind: 'network', text: 'A device on your network that has already probed a decoy', probing: true }, steps: [admin], prize: { kind: 'router', where: '192.168.1.1' }, fix: { key: admin.key, text: 'Turn on the service’s own login, or make it listen on localhost only.' }, breaks: 2 },
  { id: 'network|this-computer', entry: { kind: 'network', text: 'Any device on your network' }, steps: [ssh], prize: { kind: 'computer', where: 'this-computer' }, fix: { key: ssh.key, text: 'Set up key login for each account that needs SSH, then turn password login off.' }, breaks: 1 },
  { id: 'network|192.168.1.30', entry: { kind: 'network', text: 'Any device on your network' }, steps: [telnet], prize: { kind: 'important', where: '192.168.1.30', name: 'Garage camera' }, fix: { key: telnet.key, text: 'Turn Telnet off on the device.' }, breaks: 1, all_accepted: true },
].slice(0, 3)

test('the ways in fit the screen and read top to bottom', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: { '/netaudit/overview': { ...base, attack_paths: PATHS } } })
  await page.goto('network')
  const card = page.locator('[data-test="attack-paths"]')
  await expect(card).toBeVisible()
  await expect(card.locator('[data-test="path-step"]').first()).toContainText('Admin or metrics service')
  await expect(card.locator('[data-test="path-prize"]').first()).toHaveText(/Your router/)
  await expect(card.locator('[data-test="path-fix"]').first()).toContainText('This one fix breaks 2 of these paths.')
  await expectNoSidewaysScroll(page)
  await lookAt(card, testInfo, 'attack-paths')
})

test('an accepted path is kept and says so in words', async ({ page }) => {
  await mockAdminApi(page, { answers: { '/netaudit/overview': { ...base, attack_paths: [{ ...PATHS[0], steps: [telnet], all_accepted: true }] } } })
  await page.goto('network')
  await expect(page.locator('[data-test="path-step"]')).toContainText('accepted')
  await expectNoSidewaysScroll(page)
})

test('with nothing found it says so, and with the module off the card is not there', async ({ page }) => {
  await mockAdminApi(page, { answers: { '/netaudit/overview': { ...base, attack_paths: [] } } })
  await page.goto('network')
  await expect(page.locator('[data-test="attack-paths-none"]')).toContainText('No way in found')
  await mockAdminApi(page, { answers: { '/netaudit/overview': { ...base, attack_paths: [], attack_paths_on: false } } })
  await page.reload()
  await expect(page.locator('[data-test="posture"]')).toBeVisible()
  await expect(page.locator('[data-test="attack-paths"]')).toHaveCount(0)
})
