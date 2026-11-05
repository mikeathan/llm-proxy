import { expect, test, type Page } from '@playwright/test'
import { expectNoSidewaysScroll, lookAt, mockAdminApi, setNetauditView } from './support/adminApi'

// Network → Devices → Router DNS check: off, nothing compared, compared, nothing comparable, and internet or local network access off. At both
// widths nothing may scroll sideways, the touch targets are 44 px on a phone, and the long explanation stays folded until opened.
const scope = { ranges: [{ cidr: '192.168.1.0/24', devices: 254 }], allow_unconfined: false, device_timeout_minutes: 0, suggestions: [] }
const NEVER = '0001-01-01T00:00:00Z'
const when = new Date(Date.now() - 2 * 86400_000).toISOString()
const recent = new Date(Date.now() - 12 * 60_000).toISOString()
const differences = [
  { at: recent, name: 'dns.google', outcome: 'differ', overlap: true, router: ['10.0.0.1', '8.8.8.8'], outside: ['8.8.8.8', '8.8.4.4'] },
  { at: when, name: 'one.one.one.one', outcome: 'differ', router: [], outside: ['1.1.1.1', '1.0.0.1'] },
]
const base = {
  interval_minutes: 60, allowed_intervals: [15, 30, 60, 120, 360],
  names: ['dns.google', 'one.one.one.one', 'dns.quad9.net'], default_names: ['dns.google', 'one.one.one.one', 'dns.quad9.net'], server: '',
  resolver_url: 'https://cloudflare-dns.com/dns-query', default_resolver_url: 'https://cloudflare-dns.com/dns-query',
}
const none = { comparisons: 0, matches: 0, differs: 0, cannot_compare: 0, overlapping: 0, first: NEVER, last: NEVER, differences: [] }
const STATES = {
  off: { ...base, enabled: false, status: { last_run: NEVER }, summary: none },
  never: { ...base, enabled: true, status: { last_run: NEVER }, summary: none },
  compared: { ...base, enabled: true, status: { last_run: when }, summary: { ...none, comparisons: 48, matches: 47, differs: 1, cannot_compare: 2, overlapping: 1, first: when, last: when } },
  alive: { ...base, enabled: true, status: { last_run: recent }, summary: { ...none, comparisons: 48, matches: 48, first: when, last: recent } },
  differing: { ...base, enabled: true, status: { last_run: recent }, summary: { ...none, comparisons: 48, matches: 46, differs: 8, overlapping: 1, first: when, last: recent, differences } },
  nothingComparable: { ...base, enabled: true, status: { last_run: when }, summary: { ...none, cannot_compare: 3, first: when, last: when } },
  internetOff: { ...base, enabled: true, status: { last_run: when, problem: 'internet_off' }, summary: none },
  lanOff: { ...base, enabled: true, status: { last_run: when, problem: 'lan_off' }, summary: none },
} as const

async function open(page: Page, view: 'simple' | 'advanced', dns: object) {
  await setNetauditView(page, view)
  await mockAdminApi(page, { answers: { '/netaudit/scope': scope, '/netaudit/dns-check': dns } })
  await page.goto('network/settings')
  await expect(page.getByRole('heading', { name: 'Router DNS check' })).toBeVisible()
}

for (const view of ['simple', 'advanced'] as const) {
  for (const name of Object.keys(STATES) as (keyof typeof STATES)[]) {
    test(`router DNS check card, ${name} (${view}), fits the screen`, async ({ page }, testInfo) => {
      await open(page, view, STATES[name])
      const card = page.locator('[data-test="dns-check"]')
      await expect(card.locator('[data-test="module-switch-state"]')).toBeVisible()
      await expect(card.getByRole('switch')).toHaveCount(0)
      const status = card.locator('[data-test="dns-check-status"]')
      if (name === 'off') await expect(status).toHaveCount(0)
      else await expect(status).toBeVisible()
      if (name === 'compared') {
        await expect(status).toContainText('Compared 48 times')
        await expect(card.locator('[data-test="dns-check-fresh"]')).toContainText('may have stopped')
      }
      if (name === 'alive') await expect(card.locator('[data-test="dns-check-fresh"]')).toContainText('Last checked 12 minutes ago')
      if (name === 'differing') {
        await expect(card.locator('[data-test="dns-check-difference"]')).toHaveCount(2)
        await expect(card.locator('[data-test="dns-check-differences"]')).toContainText('6 more differences are recorded')
      }
      if (name === 'nothingComparable') await expect(status).toContainText('does not mean the answers agree')
      if (name === 'internetOff' || name === 'lanOff') {
        await expect(status).toContainText(name === 'lanOff' ? 'Local network access is off' : 'nothing was sent')
        const link = card.locator('[data-test="dns-check-settings"]')
        await expect(link).toBeVisible()
        if (testInfo.project.name === 'mobile') expect((await link.boundingBox())?.height).toBeGreaterThanOrEqual(44)
      }

      const more = card.getByRole('button', { name: 'More about this' })
      await expect(card.locator('[data-test="toggle-details-text"]')).toBeHidden()
      if (testInfo.project.name === 'mobile') {
        expect((await more.boundingBox())?.height).toBeGreaterThanOrEqual(44)
        if (view === 'advanced') {
          for (const id of ['dns-check-interval', 'dns-check-server', 'dns-check-url', 'dns-check-save', 'dns-check-copy']) {
            expect((await card.locator(`[data-test="${id}"]`).boundingBox())?.height, id).toBeGreaterThanOrEqual(44)
          }
        }
      }
      await more.click()
      await expect(card.locator('[data-test="toggle-details-text"]')).toContainText('learns which names are asked')
      await expectNoSidewaysScroll(page)
      await lookAt(page.getByRole('region', { name: 'Router DNS check' }), testInfo, `dns-check-${name}-${view}`)
    })
  }
}
