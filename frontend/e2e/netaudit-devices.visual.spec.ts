import { expect, test, type Page } from '@playwright/test'
import { expectNoSidewaysScroll, lookAt, mockAdminApi, setNetauditView } from './support/adminApi'

// Network → Devices → Devices on your network, in the states that matter: a new device waiting for a decision, the
// router's hardware address changed, and a check that cannot look. At both widths and in Simple and Advanced nothing may
// scroll sideways and every action is a comfortable touch target. The screenshots are for looking at.
const scope = { ranges: [{ cidr: '192.168.1.0/24', devices: 254 }], allow_unconfined: false, device_timeout_minutes: 0, suggestions: [] }
const seen = new Date(Date.now() - 5 * 60_000).toISOString()
const device = (over: object) => ({ mac: 'aa:bb:cc:00:00:01', ip: '192.168.1.5', first_seen: seen, last_seen: seen, approved: true, randomized: false, new: false, ...over })
const presence = { enabled: true, interval_minutes: 10, allowed_intervals: [5, 10, 15, 30], last_check: seen }
const STATES = {
  'new-device': {
    devices: { devices: [device({ mac: 'aa:bb:cc:00:00:09', ip: '192.168.1.77', approved: false, new: true }), device({ name: 'Printer' }), device({ mac: 'da:a1:19:2c:00:09', ip: '192.168.1.30', randomized: true })], gateway: { ip: '192.168.1.1', mac: 'aa:bb:cc:00:00:02', changed: false }, baselined: true, updated_at: seen },
    presence,
  },
  'router-changed': {
    devices: { devices: [device({ name: 'Printer' })], gateway: { ip: '192.168.1.1', mac: 'aa:bb:cc:00:00:99', changed: true }, baselined: true, updated_at: seen },
    presence,
  },
  'cannot-look': {
    devices: { devices: [], gateway: { changed: false }, baselined: false, updated_at: '0001-01-01T00:00:00Z' },
    presence: { ...presence, last_check: undefined, problem: 'this computer cannot list the devices on its network (only Linux can)' },
  },
  // A name a device gave when asked: information on a new device, one tap on an approved one.
  suggested: {
    devices: { devices: [device({ mac: 'aa:bb:cc:00:00:09', ip: '192.168.1.77', approved: false, new: true, suggested_name: 'Samsung' }), device({ ip: '192.168.1.8', suggested_name: 'example-laptop-with-a-long-name' })], gateway: { ip: '192.168.1.1', mac: 'aa:bb:cc:00:00:02', changed: false }, baselined: true, updated_at: seen },
    presence,
  },
  // Unnamed devices: the open ports from the last scan help tell them apart, and the prompt opens the naming list.
  unnamed: {
    devices: { devices: [device({ ip: '192.168.1.5', open_ports: [22, 80, 8080], ports_scanned_at: seen }), device({ mac: 'aa:bb:cc:00:00:02', ip: '192.168.1.6', open_ports: [], ports_scanned_at: seen }), device({ mac: 'aa:bb:cc:00:00:03', ip: '192.168.1.7', name: 'Printer' })], gateway: { changed: false }, baselined: true, updated_at: seen },
    presence,
  },
  // The computer refuses the IPv6 echo: the plain words and the two long commands must fit a phone.
  'ipv6-refused': {
    devices: { devices: [device({ name: 'Printer' })], gateway: { changed: false }, baselined: true, updated_at: seen },
    presence,
    ipv6: { enabled: true, problem: 'not_permitted' },
  },
} as const

async function open(page: Page, view: 'simple' | 'advanced', state: (typeof STATES)[keyof typeof STATES]) {
  await setNetauditView(page, view)
  await mockAdminApi(page, { answers: { '/netaudit/scope': scope, '/netaudit/devices': state.devices, '/netaudit/presence': state.presence, '/netaudit/ipv6-discovery': 'ipv6' in state ? state.ipv6 : { enabled: false } } })
  await page.goto('network/devices')
  await expect(page.getByRole('heading', { name: 'Devices on your network' })).toBeVisible()
}

for (const view of ['simple', 'advanced'] as const) {
  for (const name of Object.keys(STATES) as (keyof typeof STATES)[]) {
    test(`devices card, ${name} (${view}), fits the screen`, async ({ page }, testInfo) => {
      await open(page, view, STATES[name])
      await expectNoSidewaysScroll(page)
      const card = page.locator('[data-test="devices"]')
      if (testInfo.project.name === 'mobile') {
        // Every segmented control on this screen is a touch target: the Simple/Advanced switch and the IPv4/IPv6 tabs.
        for (const group of [page.getByRole('radiogroup', { name: 'How much detail' }).first(), card.getByRole('radiogroup', { name: 'Address type' })]) {
          if (await group.count()) expect((await group.getByRole('radio').first().boundingBox())?.height).toBeGreaterThanOrEqual(44)
        }
      }
      if (name === 'new-device') {
        await expect(card.getByText('New', { exact: true })).toBeVisible()
        const claim = card.getByRole('button', { name: "It's mine" })
        await expect(claim).toBeVisible()
        if (testInfo.project.name === 'mobile') expect((await claim.boundingBox())?.height).toBeGreaterThanOrEqual(44)
        if (view === 'advanced') await expect(card.getByText('aa:bb:cc:00:00:09')).toBeVisible()
        else await expect(card.getByText('aa:bb:cc:00:00:09')).toHaveCount(0)
      }
      if (name === 'suggested') {
        await expect(card.getByText(/It calls itself “Samsung”/)).toBeVisible()
        const use = card.getByRole('button', { name: 'Use this name' })
        await expect(use).toHaveCount(1)
        if (testInfo.project.name === 'mobile') expect((await use.boundingBox())?.height).toBeGreaterThanOrEqual(44)
        const reject = card.getByRole('button', { name: 'Not this' }).first()
        await expect(reject).toBeVisible()
        if (testInfo.project.name === 'mobile') expect((await reject.boundingBox())?.height).toBeGreaterThanOrEqual(44)
      }
      if (name === 'unnamed') {
        await expect(card.getByText('2 devices have no name')).toBeVisible()
        const show = card.getByRole('button', { name: 'Name them' })
        if (testInfo.project.name === 'mobile') expect((await show.boundingBox())?.height).toBeGreaterThanOrEqual(44)
        // The device list starts folded when nothing is new; Name them opens it on the unnamed devices alone.
        await show.click()
        await expect(card.getByText('Open ports: 22, 80, 8080')).toBeVisible()
        await expect(card.getByText('No open ports')).toBeVisible()
        await expect(card.getByText('Printer')).toHaveCount(0)
        await expectNoSidewaysScroll(page)
      }
      if (name === 'ipv6-refused') {
        await expect(card.locator('[data-test="ipv6-problem"]')).toBeVisible()
        await expect(card.getByText('sudo sysctl -w net.ipv4.ping_group_range')).toBeVisible()
        const toggle = card.getByRole('switch', { name: 'Look for IPv6 devices' })
        // The input is visually hidden; the touch target is the row that holds it.
        const row = toggle.locator('xpath=ancestor::div[contains(@class, "min-h-11")][1]')
        if (testInfo.project.name === 'mobile') expect((await row.boundingBox())?.height).toBeGreaterThanOrEqual(44)
      }
      if (name === 'router-changed') {
        const confirm = card.getByRole('button', { name: /confirm/i })
        await expect(confirm).toBeVisible()
        if (testInfo.project.name === 'mobile') expect((await confirm.boundingBox())?.height).toBeGreaterThanOrEqual(44)
      }
      if (name === 'cannot-look') await expect(card.getByText('Cannot look right now')).toBeVisible()
      await lookAt(page.getByRole('region', { name: 'Devices on your network' }), testInfo, `devices-${name}-${view}`)
    })
  }
}
