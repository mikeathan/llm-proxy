import { expect, test, type Page } from '@playwright/test'
import { expectNoSidewaysScroll, lookAt, mockAdminApi, setNetauditView } from './support/adminApi'

// Network → Devices → Nearby, and its switch on the Settings tab. At phone and desktop width, in a dark and a light theme,
// nothing scrolls sideways, touch targets on a phone are 44 px, a row opens its details (beside the list on a wide screen, in a
// sheet on a phone), and a hostile device name stays text.
const scope = { ranges: [{ cidr: '192.168.50.0/24', devices: 254 }], allow_unconfined: false, device_timeout_minutes: 0, suggestions: [] }
const NOW = new Date().toISOString()
const OLD = new Date(Date.now() - 3 * 86400_000).toISOString()
const HOSTILE = '<img src=x onerror=alert(1)>‮gpj.exe and a very long name that must wrap inside the row instead of pushing the page sideways'
const guide = { steps: ['Find the device with the address shown.', 'Switch it to access-point mode.'], risk: 'It may change its address.', undo: 'Put it back in router mode.' }
const device = (over: object) => ({ first_heard: OLD, last_heard: NOW, known: true, new: false, ...over })
const devices = [
  device({ ip: '192.168.50.7', gateway: true, names: ['Upstairs box'], upnp_type: 'InternetGatewayDevice:1', maker: 'Example Networks', model: 'EN 300', sources: ['ssdp'], services: [], mac: 'a4:00:00:00:00:11',
    problem: { check: 'rogue_gateway', title: 'A second router-like device', text: 'This device says it is an internet gateway, which means it offers to open internet ports for others, and it is not your router.', guide } }),
  device({ ip: '192.168.50.44', names: [HOSTILE], services: ['_ssh._tcp', '_smb._tcp'], ports: [22, 445], sources: ['mdns'], known: false, new: true }),
  device({ ip: '192.168.50.63', names: ['Living room TV'], services: ['_airplay._tcp', '_googlecast._tcp', '_raop._tcp', '_spotify-connect._tcp'], upnp_type: 'MediaRenderer:1', sources: ['mdns', 'ssdp'], mac: '8c:00:00:00:00:2a' }),
  device({ ip: '192.168.50.20', names: ['Office printer'], services: ['_ipp._tcp', '_printer._tcp'], ports: [631], sources: ['mdns'], mac: '3c:00:00:00:00:f0' }),
  device({ ip: '192.168.50.127', names: ['Android_a1b2c3'], services: ['_companion-link._tcp'], sources: ['mdns'], last_heard: OLD }),
]
const STATES = {
  off: { enabled: false, running: false, devices: [] },
  never: { enabled: true, running: false, devices: [] },
  running: { enabled: true, running: true, last_look: NOW, last_outcome: 'heard', devices },
  nothing: { enabled: true, running: false, last_look: NOW, last_outcome: 'nothing_heard', devices: [] },
  failed: { enabled: true, running: false, last_look: NOW, last_outcome: 'lan_off', devices: [] },
  heard: { enabled: true, running: false, last_look: NOW, last_outcome: 'heard', devices },
} as const

const SCHEMES = { 'retro-dark': 'dark', 'retro-paper': 'light' } as const
async function storeTheme(page: Page, id: keyof typeof SCHEMES) {
  await page.addInitScript(([id2, scheme]) => {
    const envelope = (data: unknown) => JSON.stringify({ version: 1, data })
    localStorage.setItem('admin-ui:theme-selection', envelope({ kind: 'preset', id: id2 }))
    localStorage.setItem('admin-ui:theme-applied', envelope({ themeId: id2, presetId: id2, scheme, overrides: {} }))
  }, [id, SCHEMES[id]] as const)
}

async function open(page: Page, view: 'simple' | 'advanced', nearby: object, theme: keyof typeof SCHEMES = 'retro-dark') {
  await storeTheme(page, theme)
  await setNetauditView(page, view)
  await mockAdminApi(page, { answers: { '/netaudit/scope': scope, '/netaudit/nearby': nearby } })
  await page.goto('network/devices')
  await expect(page.getByRole('heading', { name: 'Nearby', exact: true })).toBeVisible()
}

for (const theme of Object.keys(SCHEMES) as (keyof typeof SCHEMES)[]) {
  for (const name of Object.keys(STATES) as (keyof typeof STATES)[]) {
    test(`nearby, ${name} (${theme}), fits the screen`, async ({ page }, testInfo) => {
      await open(page, 'simple', STATES[name], theme)
      const card = page.locator('[data-test="nearby"]')
      if (name === 'off') {
        await expect(card.locator('[data-test="nearby-off"]')).toContainText('two small questions')
        const link = card.locator('[data-test="nearby-turn-on"]')
        await expect(link).toBeVisible()
        if (testInfo.project.name === 'mobile') expect((await link.boundingBox())?.height).toBeGreaterThanOrEqual(44)
      } else {
        await expect(card.locator('[data-test="nearby-summary"]')).toBeVisible()
        const check = card.locator('[data-test="nearby-check"]')
        if (testInfo.project.name === 'mobile') expect((await check.boundingBox())?.height).toBeGreaterThanOrEqual(44)
      }
      if (name === 'nothing') await expect(card).toContainText('does not mean nothing is there')
      if (name === 'failed') await expect(card).toContainText('Guardrails')
      if (name === 'running') await expect(card.locator('[data-test="nearby-check"]')).toBeDisabled()
      await expectNoSidewaysScroll(page)
      await lookAt(page, testInfo, `nearby-${name}-${theme}`, true)
    })
  }

  test(`nearby list: pinned problem, rows, details and a hostile name (${theme})`, async ({ page }, testInfo) => {
    await open(page, 'simple', STATES.heard, theme)
    const card = page.locator('[data-test="nearby"]')
    await expect(card.locator('[data-test="nearby-pinned"]')).toContainText('not your router')
    await expect(card.locator('[data-test="nearby-row"]')).toHaveCount(5)
    await expect(card.locator('[data-test="nearby-row"]').first()).toContainText('Upstairs box')
    await expect(page.locator('img[src="x"]')).toHaveCount(0)
    const hostile = card.locator('[data-test="nearby-name"]', { hasText: 'very long name' })
    await expect(hostile).toContainText('<img src=x')
    const mobile = testInfo.project.name === 'mobile'
    if (mobile) for (const row of await card.locator('[data-test="nearby-row"]').all()) expect((await row.boundingBox())?.height).toBeGreaterThanOrEqual(44)

    await card.locator('[data-test="nearby-row"]').first().click()
    const detail = page.locator(mobile ? '[data-test="nearby-sheet"]' : '[data-test="nearby-panel"]')
    await expect(detail).toBeVisible()
    await detail.locator('[data-test="nearby-fix"]').click()
    await expect(detail).toContainText('Switch it to access-point mode.')
    await expectNoSidewaysScroll(page)
    await lookAt(page, testInfo, `nearby-open-${theme}`)
    if (mobile) {
      expect((await detail.locator('[data-test="nearby-fix"]').boundingBox())?.height).toBeGreaterThanOrEqual(44)
      await detail.locator('[data-test="nearby-close"]').click()
      await expect(detail).toHaveCount(0)
    }
  })

  test(`nearby advanced table and filter (${theme})`, async ({ page }, testInfo) => {
    await open(page, 'advanced', STATES.heard, theme)
    const card = page.locator('[data-test="nearby"]')
    if (testInfo.project.name === 'mobile') {
      // A phone shows the rows in Advanced too: the table's service column was a letter or two wide there (vertex, 2026-11-05).
      await expect(card.locator('[data-test="nearby-table"]')).toHaveCount(0)
      await expect(card.locator('[data-test="nearby-row"]').first()).toBeVisible()
    } else {
      await expect(card.locator('[data-test="nearby-table"]')).toContainText('_ipp._tcp, _printer._tcp')
      await expect(card.locator('[data-test="nearby-table"]')).toContainText('InternetGatewayDevice:1')
    }
    await expect(card.locator('[data-test="nearby-copy"]')).toBeVisible()
    await expectNoSidewaysScroll(page)
    await lookAt(page, testInfo, `nearby-advanced-${theme}`, true)
  })
}

test('the Nearby switch lives on the Settings tab and says what it sends', async ({ page }) => {
  await storeTheme(page, 'retro-dark')
  await mockAdminApi(page, { answers: { '/netaudit/scope': scope, '/netaudit/nearby': STATES.off } })
  await page.goto('network/settings')
  const card = page.locator('[data-test="nearby-card"]')
  await expect(card.getByRole('switch', { name: 'Ask the network what each device is' })).toBeVisible()
  await card.getByRole('button', { name: 'More about this' }).click()
  await expect(card.locator('[data-test="toggle-details-text"]')).toContainText('asks for your passphrase')
  await expectNoSidewaysScroll(page)
})
