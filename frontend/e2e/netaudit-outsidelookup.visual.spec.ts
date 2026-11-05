import { expect, test, type Page } from '@playwright/test'
import { expectNoSidewaysScroll, lookAt, mockAdminApi, setNetauditView } from './support/adminApi'

// Network → Devices → Internet view: off, never looked, looked, could not look and could not reach the service. At both widths
// nothing may scroll sideways, the touch targets are 44 px on a phone, and the long explanation stays folded until opened.
const scope = { ranges: [{ cidr: '192.168.1.0/24', devices: 254 }], allow_unconfined: false, device_timeout_minutes: 0, suggestions: [] }
const at = new Date(Date.now() - 3 * 3600_000).toISOString()
const base = { url: 'https://internetdb.shodan.io', default_url: 'https://internetdb.shodan.io', address: '' }
const STATES = {
  off: { enabled: false, ...base, last: { at: '0001-01-01T00:00:00Z', ports: 0 } },
  never: { enabled: true, ...base, last: { at: '0001-01-01T00:00:00Z', ports: 0 } },
  looked: { enabled: true, ...base, last: { at, outcome: 'looked', ports: 2 } },
  upnpOff: { enabled: true, ...base, address: '', last: { at, outcome: 'cannot_look', reason: 'upnp_off', ports: 0 } },
  entered: { enabled: true, ...base, address: '203.0.113.7', last: { at, outcome: 'looked', ports: 1, source: 'entered', differs: true } },
  private: { enabled: true, ...base, last: { at, outcome: 'cannot_look', reason: 'private_address', ports: 0 } },
  internetOff: { enabled: true, ...base, last: { at, outcome: 'failed', reason: 'internet_off', ports: 0 } },
  failed: { enabled: true, ...base, last: { at, outcome: 'failed', reason: 'lookup_failed', ports: 0 } },
} as const

async function open(page: Page, view: 'simple' | 'advanced', outside: object) {
  await setNetauditView(page, view)
  await mockAdminApi(page, { answers: { '/netaudit/scope': scope, '/netaudit/outside-lookup': outside } })
  await page.goto('network/settings')
  await expect(page.getByRole('heading', { name: 'Internet view' })).toBeVisible()
}

for (const view of ['simple', 'advanced'] as const) {
  for (const name of Object.keys(STATES) as (keyof typeof STATES)[]) {
    test(`internet view card, ${name} (${view}), fits the screen`, async ({ page }, testInfo) => {
      await open(page, view, STATES[name])
      const card = page.locator('[data-test="outside-lookup"]')
      await expect(card.locator('[data-test="module-switch-state"]')).toBeVisible()
      await expect(card.getByRole('switch')).toHaveCount(0)
      const status = card.locator('[data-test="outside-lookup-status"]')
      if (name === 'off') await expect(status).toHaveCount(0)
      else await expect(status).toBeVisible()
      if (name === 'looked') await expect(status).toContainText('2 open ports')
      if (name === 'private') await expect(status).toContainText('Nothing was sent')
      const address = card.locator('[data-test="outside-lookup-address"]')
      if (name === 'upnpOff' || name === 'private' || name === 'entered' || (view === 'advanced' && name !== 'off')) await expect(address).toBeVisible()
      else await expect(address).toHaveCount(0)
      if (name === 'entered') {
        await expect(status).toContainText('at the address you entered')
        await expect(card.locator('[data-test="outside-lookup-differs"]')).toContainText('different public address')
      }
      if (name === 'upnpOff' && testInfo.project.name === 'mobile') {
        expect((await address.boundingBox())?.height).toBeGreaterThanOrEqual(44)
        expect((await card.locator('[data-test="outside-lookup-address-save"]').boundingBox())?.height).toBeGreaterThanOrEqual(44)
      }
      if (name === 'internetOff') {
        await expect(status).toContainText('internet access is off')
        const link = card.locator('[data-test="outside-lookup-settings"]')
        await expect(link).toBeVisible()
        if (testInfo.project.name === 'mobile') expect((await link.boundingBox())?.height).toBeGreaterThanOrEqual(44)
      }

      const more = card.getByRole('button', { name: 'More about this' })
      await expect(card.locator('[data-test="toggle-details-text"]')).toBeHidden()
      if (testInfo.project.name === 'mobile') {
        expect((await more.boundingBox())?.height).toBeGreaterThanOrEqual(44)
        if (view === 'advanced') {
          expect((await card.locator('[data-test="outside-lookup-url"]').boundingBox())?.height).toBeGreaterThanOrEqual(44)
          expect((await card.locator('[data-test="outside-lookup-save"]').boundingBox())?.height).toBeGreaterThanOrEqual(44)
        }
      }
      await more.click()
      await expect(card.locator('[data-test="toggle-details-text"]')).toContainText('learns your public internet address')
      await expectNoSidewaysScroll(page)
      await lookAt(page.getByRole('region', { name: 'Internet view' }), testInfo, `internet-view-${name}-${view}`)
    })
  }
}
