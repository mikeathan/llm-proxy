import { expect, test } from '@playwright/test'
import { expectNoSidewaysScroll, expectTapArea, lookAt, mockAdminApi, setNetauditView } from './support/adminApi'

// The Radio page's tabs: Listening, Around you (the radar, the kinds of device, the channels), My devices (with your networks and where they are), Sensors. At both widths nothing scrolls
// sideways and on a phone every button is a 44 px target. The data is invented; the shape is the server's.
const WIN = (channel: number, frames: number, stop = 'window') => ({
  channel, frames, recorded: frames, own_frames: 0, unread: 0, stop, kernel_seen: frames, kernel_dropped: 0, elapsed_ms: 20000,
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
const AIR_DEVICES = {
  listened_cycles: 30, min_listened_cycles: 12, total: 2,
  devices: [
    { id: 'a1b2c3d4', class: 'neighbour', why: 'never_strong', signal: 'weak', first_seen: '2026-11-12', last_seen: '2026-11-14', cycles_heard: 20, need_heard: 3, retry_percent: 12, channels: [1, 11, 60], random_address: false, joined_own: false },
    { id: 'e5f6a7b8', class: 'neighbour', why: 'never_strong', signal: 'weak', first_seen: '2026-11-13', last_seen: '2026-11-14', cycles_heard: 9, need_heard: 3, retry_percent: 31, channels: [6], random_address: true, joined_own: false },
  ],
}
const AIRWAVES = {
  channels: [{ channel: 1, devices: 4 }, { channel: 6, devices: 12 }, { channel: 60, devices: 3 }],
  classes: [{ class: 'infrastructure', devices: 4, frames: 1000, retry_percent: 8 }, { class: 'neighbour', devices: 9, frames: 400, retry_percent: 31 }],
  routers: [{ name: 'netsink', channel: 60 }], near_unknown: 2, listened_cycles: 30,
}
const LIVE = {
  listening: true, cycles: 6, model: { ref_24_dbm: -40, ref_5_dbm: -47, calibrated: false, provisional: true },
  rings: { room: 2, house: 2, nearby: 2 }, cycle_seconds: 300, last_cycle_at: Math.floor(Date.now() / 1000) - 100, listened_cycles: 5, min_listened_cycles: 12,
  devices: [
    { id: 'ab', mine: true, label: 'my phone', class: 'personal', dbm: -48, near_m: 1.4, far_m: 3.2, ring: 'room', angle: 40, trend: 'steady', channel: 6, heard: true, tail_m: [2.4, 2.2, 2.3, 2.2] },
    { id: 'c0ffee11', mine: false, class: 'infrastructure', dbm: -55, near_m: 2.1, far_m: 6, ring: 'room', angle: 130, trend: 'steady', channel: 6, heard: true, network: 'netsink-2G', last_at: Math.floor(Date.now() / 1000) - 40, tail_m: [4, 4, 3.9, 4],
      detail: { id: 'c0ffee11', class: 'infrastructure', why: 'always_here_steady_signal', signal: 'strong', first_seen: '2026-11-06', last_seen: '2026-11-14', cycles_heard: 118, need_heard: 12, retry_percent: 0, channels: [6], random_address: false, joined_own: false, maker: 'TP-Link', wifi_generation: 5 } },
    { id: 'a1b2c3d4', mine: false, class: 'unknown', dbm: -62, near_m: 3, far_m: 11, ring: 'house', angle: 215, trend: 'closer', channel: 60, heard: true, unusual: true, usual_dbm: -78, shift_db: 16,
      tail_m: [14, 11, 8, 5, 3.5], detail: { id: 'a1b2c3d4', class: 'unknown', why: 'needs_more_listening', signal: 'weak', first_seen: '2026-11-12', last_seen: '2026-11-14', cycles_heard: 2, need_heard: 3, retry_percent: 12, channels: [1, 11, 60], random_address: false, joined_own: false } },
    { id: 'e5f6a7b8', mine: false, class: 'neighbour', dbm: -70, near_m: 5, far_m: 24, ring: 'house', angle: 300, trend: 'away', channel: 11, heard: false },
    { id: '99aa00bb', mine: false, class: 'neighbour', last_at: Math.floor(Date.now() / 1000) - 300, dbm: -78, near_m: 8, far_m: 48, ring: 'nearby', angle: 75, trend: 'unknown', channel: 1, heard: true },
    { id: '12ab34cd', mine: false, class: 'transient', dbm: -84, near_m: 14, far_m: 100, ring: 'nearby', angle: 255, trend: 'unknown', channel: 36, heard: true },
    { id: 'beefcafe', mine: true, label: 'my laptop', class: 'infrastructure', dbm: -52, near_m: 1.7, far_m: 4.5, ring: 'room', angle: 170, trend: 'unknown', channel: 36, heard: false, usual: true },
    { id: '0badf00d', mine: false, class: 'infrastructure', dbm: -66, near_m: 4, far_m: 16, ring: 'house', angle: 20, trend: 'unknown', channel: 6, heard: false, usual: true },
    { id: 'feedbeef', mine: false, class: 'neighbour', dbm: -81, near_m: 10, far_m: 60, ring: 'nearby', angle: 330, trend: 'unknown', channel: 1, heard: false, usual: true },
  ],
}
const TRACK = {
  id: 'a1b2c3d4', mine: false, trend: 'closer', stored: true, now: 1_800_000_000,
  points: Array.from({ length: 40 }, (_, i) => ({ at: 1_800_000_000 - (40 - i) * 600 - (i > 22 ? 5400 : 0), dbm: -80 + i, near_m: 12 - i * 0.2, far_m: 40 - i * 0.7, run: i > 22 ? 1 : 0 })),
}
const SCAN = {
  plan: { channels: [1, 6, 11, 36, 60, 149], window_seconds: 20, cycle_seconds: 300 }, default: { channels: [1, 6, 11, 36, 60, 149], window_seconds: 20, cycle_seconds: 300 },
  custom: false, windows_per_hour: 72, hours: 168, default_hours: 168,
  limits: { channels: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 36, 40, 44, 48, 52, 56, 60, 64, 100, 104, 108, 112, 116, 120, 124, 128, 132, 136, 140, 149, 153, 157, 161, 165], max_channels: 16, min_window_seconds: 1, max_window_seconds: 30, min_cycle_seconds: 60, max_windows_per_hour: 200, max_hours: 168 },
}
const HOURS = Array.from({ length: 48 }, (_, i) => ({ hour: new Date(Date.UTC(2026, 10, 13, 15 + i)).toISOString(), state: i < 20 ? 'not_listening' : i % 5 === 0 ? 'away' : 'here', signal: 'strong', ...(i >= 20 && i % 5 !== 0 ? { dbm: -48 - (i % 7) * 3, near_m: 1.4 + (i % 7) * 0.5, far_m: 3.2 + (i % 7) * 2 } : {}) }))
const PRESENCE = { devices: [{ id: 'ab', label: 'my phone', state: 'here', signal: 'strong', last_heard: '2026-11-14T15:00:00Z', joined_own: true, hours: HOURS }] }
const DIGEST = {
  listening: { cycles: 30, min_cycles: 12, window_days: 7, cycles_last_24h: 30, now: true, hours_left: 100 }, classifier: { version: 2, provisional: true }, own: { devices: 1, access_points: 2, confirmed_access_points: 1 }, others: [],
  findings: [{ id: 'radio_pmf_off', tier: 'seen', count: 1, claim: 'Confirmed WPA2 access points of yours that do not protect their management frames, so a device can be disconnected by a forged frame.', means: 'Common on home routers and low risk on its own: someone nearby could disconnect a device, not read what it sends.' }],
  could_not_see: [{ id: 'radio_hidden_ap_near', reason: 'A beacon’s signal strength is not kept per access point, so how close a hidden network is cannot be told.' }],
}
const ANSWERS = {
  '/radio/digest': DIGEST,
  '/radio/sensors': SENSORS, '/radio/listening': LISTENING, '/radio/air': AIR, '/radio/air/devices': AIR_DEVICES, '/radio/air/airwaves': AIRWAVES, '/radio/air/live': LIVE, '/radio/air/track/a1b2c3d4': TRACK, '/radio/listening/settings': SCAN,
  '/radio/devices/mine': DEVICES, '/radio/devices/mine/presence': PRESENCE, '/radio/networks': NETWORKS,
}

async function touchTargets(page: import('@playwright/test').Page, testInfo: import('@playwright/test').TestInfo) {
  if (testInfo.project.name !== 'mobile') return
  for (const b of await page.locator('main button, main [role="button"]').all()) {
    if (!(await b.isVisible())) continue
    // The Simple/Advanced control is drawn 28 px high; the area around it answers for it (a 44 px target).
    if (await b.evaluate((el) => !!el.closest('[role="radiogroup"]'))) { await b.evaluate((el) => el.scrollIntoView({ block: 'center' })); await expectTapArea(b); continue } // centred: at the screen's edge the fixed bottom bar can sit over the area under the control
    expect((await b.boundingBox())?.height).toBeGreaterThanOrEqual(44)
  }
}

test('Listening: where the run is, the cut window, and the way to who is around', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: ANSWERS })
  await page.goto('radio')
  await expect(page.locator('[data-test="radio-listen-progress"]')).toContainText('Cycle 1 in progress: 5 of 6 channels heard.')
  await expect(page.locator('[data-test="radio-window-cut"]')).toContainText('4,000-frame limit')
  await expect(page.locator('[data-test="radio-window-band"]').first()).toHaveText('5 GHz')
  await lookAt(page, testInfo, 'listening', true)
  await expect(page.locator('[data-test="radio-around-total"]')).toHaveText('18 devices around you')
  await expectNoSidewaysScroll(page)
  await touchTargets(page, testInfo)
})

test('Listening: the scan plan opens to channel chips, and a plan that cannot run is refused before Save', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: { ...ANSWERS, '/radio/listening': { ...LISTENING, listening: false } } })
  await page.goto('radio')
  await expect(page.locator('[data-test="radio-scan-summary"]')).toContainText('6 channels, 20 seconds each, every 5 minutes')
  await page.locator('[data-test="radio-scan-toggle"]').click()
  await expect(page.locator('[data-test="radio-scan-channel"]')).toHaveCount(37)
  await page.locator('[data-test="radio-scan-channel"][data-channel="165"]').click()
  await expect(page.locator('[data-test="radio-scan-check"]')).toContainText('7 channels')
  await lookAt(page.locator('[data-test="radio-scan"]'), testInfo, 'scan-plan')
  await expectNoSidewaysScroll(page)
  await touchTargets(page, testInfo)
})

test('Around you: the status line, the radar with one list, the device opened below and the channels', async ({ page }, testInfo) => {
  await setNetauditView(page, 'advanced')
  await mockAdminApi(page, { answers: ANSWERS })
  await page.goto('radio/around')
  await expect(page.locator('[data-test="radio-finding"]')).toContainText('management frames')
  await expect(page.locator('[data-test="radio-finding-means"]')).toContainText('low risk on its own')
  await expect(page.locator('[data-test="radio-report-setup"]')).toBeVisible() // the report card: no automation yet, so it asks when to run
  await expect(page.locator('[data-test="radio-report-go"]')).toHaveText('Set up and analyse')
  await expect(page.locator('[data-test="radio-status-next"]')).toContainText('Next reading in')
  await expect(page.locator('[data-test="radio-status-sorting"]')).toContainText('Sorting starts after 12 cycles (5 done)')
  await expect(page.locator('[data-test="radio-radar-summary"]')).toContainText('6 devices heard now, 1 of them yours')
  await expect(page.locator('[data-test="radio-dot"]')).toHaveCount(6) // the three usually here stay out until asked for
  await expect(page.locator('[data-test="radio-dot-label"]')).toHaveText('my phone · 1.4–3.2 m')
  await expect(page.locator('[data-test="radio-chip"]')).toHaveCount(7)
  await expect(page.locator('[data-test="radio-row-title"]').first()).toBeVisible()
  await page.locator('[data-filter="neighbour"]').click()
  await expect(page.locator('[data-test="radio-radar-row"]')).toHaveCount(2)
  await page.locator('[data-filter="all"]').click()
  await page.locator('[data-test="radio-usual-toggle"]').click()
  await expect(page.locator('[data-test="radio-radar-row"][data-usual="1"]')).toHaveCount(3)
  await expect(page.locator('[data-test="radio-dot"]')).toHaveCount(9)
  await page.locator('[data-test="radio-radar-row"]', { hasText: 'a1b2c3d4' }).locator('button').click()
  await expect(page.locator('[data-test="radio-radar-chosen"]')).toContainText('dBm on channel')
  await expect(page.locator('[data-test="radio-dot"]')).toHaveCount(1) // a picked device is drawn alone
  await expect(page.locator('[data-test="radio-air-waiting"]')).toContainText('Waiting:')
  await expect(page.locator('[data-test="radio-track"] svg')).toBeVisible()
  await expect(page.locator('[data-test="radio-track-line"]')).toHaveCount(2)
  await page.locator('[data-test="radio-guide"] summary').click()
  await expect(page.locator('[data-test="radio-around-provisional"]')).toContainText('first draft')
  await lookAt(page, testInfo, 'around-you', true)
  await expectNoSidewaysScroll(page)
  await touchTargets(page, testInfo)
})

test('Around you: what a device is (maker, Wi-Fi, network), the switches, naming one, and the other two views', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: ANSWERS })
  await page.goto('radio/around')
  await expect(page.locator('[data-test="radio-row-title"]', { hasText: 'netsink-2G' })).toBeVisible()
  await page.locator('[data-test="radio-radar-row-button"]', { hasText: 'netsink-2G' }).click()
  await expect(page.locator('[data-test="radio-detail-maker"]')).toHaveText('TP-Link')
  await expect(page.locator('[data-test="radio-detail-network"]')).toHaveText('netsink-2G')
  await expect(page.locator('[data-test="radio-detail-facts"]')).toContainText('Wi-Fi 5')
  await expect(page.locator('[data-test="radio-focus-note"]')).toContainText('Showing only netsink-2G')
  await page.locator('[data-test="radio-focus-all"]').click()
  await expect(page.locator('[data-test="radio-dot"]')).toHaveCount(6)
  await page.locator('[data-test="radio-toggle-range"]').click()
  await page.locator('[data-test="radio-toggle-spoke"]').click()
  await expect(page.locator('[data-test="radio-dot-spoke"]')).toHaveCount(6)
  await page.locator('[data-test="radio-mark-open"]').click()
  await expect(page.locator('[data-test="radio-mark-input"]')).toBeVisible()
  await page.locator('[data-test="radio-find-open"]').click()
  await expect(page.locator('[data-test="radio-find-candidate"]')).toHaveCount(3)
  await lookAt(page, testInfo, 'around-you-device', true)
  await page.locator('[data-test="radio-view-select"]').selectOption('changes')
  await expect(page.locator('[data-test="radio-view-changes"]')).toBeVisible()
  await expect(page.locator('[data-test="radio-change"]').first()).toBeVisible()
  await lookAt(page, testInfo, 'around-you-changes', true)
  await page.locator('[data-test="radio-view-select"]').selectOption('channels')
  await expect(page.locator('[data-test="radio-channels-headline"]')).toContainText('netsink is on channel 60')
  await page.locator('[data-channel="6"]').click()
  await expect(page.locator('[data-test="radio-channel-note"]')).toContainText('channel 6')
  await lookAt(page, testInfo, 'around-you-channels', true)
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
  await expect(page.locator('[data-test="radio-device-presence"]')).toContainText('Here now')
  await expect(page.locator('[data-test="radio-presence-hour"]')).toHaveCount(48)
  await expect(page.locator('[data-test="radio-presence-bar"]').first()).toBeVisible()
  await lookAt(page, testInfo, 'my-devices', true)
  await expect(page.locator('[data-test="radio-ap"][data-confirmed="false"]')).toContainText('Candidate')
  await expectNoSidewaysScroll(page)
  await touchTargets(page, testInfo)
})
