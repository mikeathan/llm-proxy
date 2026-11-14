import { expect, test } from '@playwright/test'
import { expectNoSidewaysScroll, mockAdminApi } from './support/adminApi'

// The Radio page's tabs: Listening (with what is around you), Sensors, My devices (with your networks). At both widths nothing scrolls
// sideways and on a phone every button is a 44 px target. The data is invented; the shape is the server's.
const WIN = (channel: number, frames: number, stop = 'window') => ({
  channel, frames, recorded: frames, unread: 0, stop, kernel_seen: frames, kernel_dropped: 0, elapsed_ms: 20000,
  on_channel: frames, off_channel: 0, channel_matched: true, at: '2026-11-14T15:00:00Z',
})
const CAPS = [
  { id: 'discovery', title: 'Found and described', use: 'used', known: true, note: 'Read from the system’s own records: bus, driver, mode.' },
  { id: 'monitor_capture', title: 'Listen to nearby Wi-Fi', use: 'used', known: true, note: 'Listening now, in short windows, frame headers only.' },
  { id: 'frame_injection', title: 'Send frames', use: 'unused_by_design', known: true, note: 'The adapter may be able to. The Radio stack never sends anything.' },
  { id: 'he_decode', title: 'Read Wi-Fi 6 traffic', use: 'unsupported', known: true, note: 'This adapter cannot decode Wi-Fi 6, so volumes on 5 GHz are undercounted. Joins, leaves and beacons are still complete.' },
]
const SENSORS = {
  enabled: true, unused: 2,
  sensors: [
    { id: 'wlxf0090da058a3', kind: 'wifi', bus: 'usb', driver: 'rtw88_8822bu', usb_id: '2357:012e', mode: 'monitor', eligible: true, role: 'listening',
      why: 'a USB adapter that is not carrying this computer’s connection',
      probe: { read_at: '2026-11-14', modes: ['monitor', 'managed'], bands: ['2.4 GHz', '5 GHz'], standards: ['Wi-Fi 4', 'Wi-Fi 5'], monitor: true }, capabilities: CAPS },
    { id: 'wlp2s0', kind: 'wifi', bus: 'pci', driver: 'mt7921e', mode: 'managed', eligible: false, role: 'wont_use',
      why: 'not a USB adapter: the built-in card is never used for capture', capabilities: [CAPS[0], { ...CAPS[1], use: 'unused_by_design', note: 'not a USB adapter: the built-in card is never used for capture.' }] },
    { id: 'hci0', kind: 'bluetooth', bus: 'usb', driver: 'btusb', usb_id: '0e8d:c616', eligible: false, role: 'no_driver',
      why: 'found, and nothing in the stack drives this kind of radio yet', capabilities: [{ id: 'ble_scan', title: 'Bluetooth LE scan', use: 'found_no_driver', known: false, note: 'Found. Nothing in the stack drives it yet.' }] },
  ],
}
const PLAN = { channels: [1, 6, 11, 36, 60, 149], window_seconds: 20, cycle_seconds: 300 }
const LISTENING = {
  listening: true, adapter: 'wlxf0090da058a3', until: '2026-11-14T16:26:10Z', cycles: 0, frames: 9571, plan: PLAN,
  windows: [WIN(1, 535), WIN(11, 1658), WIN(36, 652), WIN(60, 4000, 'frame_cap')],
}
const AIR = {
  rules: { version: 1, provisional: true, window_days: 7, min_cycles_listened: 12 }, listened_cycles: 5, total: 18,
  counts: { infrastructure: 3, personal: 2, neighbour: 9, transient: 1, unknown: 3 },
  changes: [{ day: '2026-11-14', from: '', to: 'neighbour', version: 1, reason: 'never_strong', n: 9 }],
}
const DEVICES = { devices: [{ id: 'ab', label: 'my phone', visits: 3, first_seen: '2026-11-12', last_seen: '2026-11-14', class: 'personal', class_by: 'owner' }] }
const NETWORKS = {
  names: ['netsink', 'netsink-2G'],
  access_points: [
    { id: 'a1', name: 'netsink', channel: 60, security: 'wpa2', confirmed: true, last_seen: '2026-11-14' },
    { id: 'a2', name: 'netsink-2G', channel: 6, security: 'wpa2', confirmed: false, last_seen: '2026-11-14' },
  ],
}
const ANSWERS = {
  '/radio/sensors': SENSORS, '/radio/listening': LISTENING, '/radio/air': AIR, '/radio/devices/mine': DEVICES, '/radio/networks': NETWORKS,
}

async function touchTargets(page: import('@playwright/test').Page, testInfo: import('@playwright/test').TestInfo) {
  if (testInfo.project.name !== 'mobile') return
  for (const b of await page.locator('main button, main [role="button"]').all()) {
    if (!(await b.isVisible())) continue
    expect((await b.boundingBox())?.height).toBeGreaterThanOrEqual(44)
  }
}

test('Listening: where the run is, the cut window, and what is around you', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: ANSWERS })
  await page.goto('radio')
  await expect(page.locator('[data-test="radio-listen-progress"]')).toContainText('Cycle 1 in progress: 5 of 6 channels heard.')
  await expect(page.locator('[data-test="radio-window-cut"]')).toContainText('4,000-frame limit')
  await expect(page.locator('[data-test="radio-around-total"]')).toHaveText('18 devices around you')
  await expect(page.locator('[data-test="radio-around-provisional"]')).toContainText('first draft')
  await expectNoSidewaysScroll(page)
  await touchTargets(page, testInfo)
})

test('Sensors: a role for each radio, the probe, and the details folded away', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: ANSWERS })
  await page.goto('radio/sensors')
  await expect(page.locator('[data-test="radio-found"]')).toHaveText('3 radios found, 1 can listen.')
  await expect(page.locator('[data-sensor="wlxf0090da058a3"] [data-test="radio-role"]')).toContainText('Listening now')
  await expect(page.locator('[data-sensor="wlp2s0"] [data-test="radio-role"]')).toContainText('Will not be used')
  await expect(page.locator('[data-sensor="wlxf0090da058a3"] [data-test="radio-probe"]')).toContainText('2.4 GHz and 5 GHz · Wi-Fi 4, Wi-Fi 5')
  await expect(page.locator('[data-test="radio-capability"]:visible')).toHaveCount(0)
  await page.locator('[data-sensor="wlxf0090da058a3"]').getByRole('button', { name: /Details/ }).click()
  await expect(page.locator('[data-sensor="wlxf0090da058a3"] [data-test="radio-capability"]:visible')).toHaveCount(4)
  await expectNoSidewaysScroll(page)
  await touchTargets(page, testInfo)
})

test('My devices: the owner’s devices with their kind, and the networks to confirm', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: ANSWERS })
  await page.goto('radio/devices')
  await expect(page.locator('[data-test="radio-device-class"]')).toHaveText('Comes and goes')
  await expect(page.locator('[data-test="radio-ap"][data-confirmed="false"]')).toContainText('Candidate')
  await expectNoSidewaysScroll(page)
  await touchTargets(page, testInfo)
})
