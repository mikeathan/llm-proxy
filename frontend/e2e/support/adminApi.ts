import { expect, type Locator, type Page, type TestInfo } from '@playwright/test'
import { DEFAULT_CONFIG } from '../../src/composables/models/useConfig'

// What every admin page asks for when it loads, so a spec names only the answers it cares about. Endpoints the pages
// read as lists answer `[]` (a `{}` there throws in the page and hides the section under test); anything unknown `{}`.
const LIST_ENDPOINTS = ['/manifests', '/mcp', '/workspaces', '/profiles', '/profile-schema', '/automations']
const READY = { ready: true, requirements: [], notices: [] }
// No devices seen and the new-device check off, the answers the Devices card needs to draw.
const DEVICES = { devices: [], gateway: { changed: false }, baselined: true, updated_at: '0001-01-01T00:00:00Z' }
const DECOYS = { enabled: false, listening: [], ports: [{ number: 2222, name: 'SSH login' }], touches: [] }
const PRESENCE = { enabled: false, interval_minutes: 10, allowed_intervals: [5, 10, 15, 30] }
const IPV6_DISCOVERY = { enabled: false }
const OUTSIDE_LOOKUP = { enabled: false, url: 'https://internetdb.shodan.io', default_url: 'https://internetdb.shodan.io', last: { at: '0001-01-01T00:00:00Z', ports: 0 } }
const KNOWN_BAD = { count: 0, has_address: false, ready: false, reason: 'no list address is saved yet' }
const NEARBY = { enabled: false, running: false, devices: [] }
const UBUNTU_DATA = { supported: true, host_release: 'noble', running: false, notices: 0, packages: 0 }
const NEVER = '0001-01-01T00:00:00Z'
const DNS_CHECK = {
  enabled: false, finding_enabled: false, interval_minutes: 60, allowed_intervals: [15, 30, 60, 120, 360],
  names: ['dns.google', 'one.one.one.one', 'dns.quad9.net'], default_names: ['dns.google', 'one.one.one.one', 'dns.quad9.net'], server: '',
  resolver_url: 'https://cloudflare-dns.com/dns-query', default_resolver_url: 'https://cloudflare-dns.com/dns-query',
  status: { last_run: NEVER },
  summary: { comparisons: 0, matches: 0, differs: 0, cannot_compare: 0, overlapping: 0, first: NEVER, last: NEVER, differences: [] },
}
// Signed in, so no spec sees the login page or the "sign-in is not set up" banner unless it asks for them.
const SIGNED_IN = { set_up: true, setup_open: false, authenticated: true, elevated: false, encrypted: true }

const DEFAULT_ANSWERS: Record<string, unknown> = {
  '/netaudit/readiness': READY,
  '/auth/status': SIGNED_IN,
  '/netaudit/devices': DEVICES,
  '/netaudit/presence': PRESENCE,
  '/netaudit/ipv6-discovery': IPV6_DISCOVERY,
  '/netaudit/decoys': DECOYS,
  '/netaudit/outside-lookup': OUTSIDE_LOOKUP,
  '/netaudit/dns-check': DNS_CHECK,
  '/netaudit/nearby': NEARBY,
  '/netaudit/known-bad': KNOWN_BAD,
  '/netaudit/ubuntu-data': UBUNTU_DATA,
}

type Answers = Record<string, unknown>

interface Options {
  /** Answers by the end of the request path, for example `'/netaudit/scope'`. They win over the defaults. */
  answers?: Answers
  /** The configuration `/state` returns; the default configuration when absent. */
  config?: typeof DEFAULT_CONFIG
}

export async function mockAdminApi(page: Page, { answers = {}, config = DEFAULT_CONFIG }: Options = {}) {
  const state = { config: structuredClone(config), models: [], available_models: [] }
  await page.route('**/admin/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    const given = Object.keys(answers).find((suffix) => path.endsWith(suffix))
    const fallback = Object.keys(DEFAULT_ANSWERS).find((suffix) => path.endsWith(suffix))
    const json = given
      ? answers[given]
      : path.endsWith('/state')
        ? state
        : fallback
          ? DEFAULT_ANSWERS[fallback]
          : LIST_ENDPOINTS.some((suffix) => path.endsWith(suffix))
            ? []
            : {}
    await route.fulfill({ json })
  })
}

/** The Simple/Advanced choice the network-scan screens remember. */
export const setNetauditView = (page: Page, view: 'simple' | 'advanced') =>
  page.addInitScript((v) => localStorage.setItem('admin-ui:netaudit-view', JSON.stringify({ version: 1, data: v })), view)

/** No page may scroll sideways, at either width. */
export async function expectNoSidewaysScroll(page: Page) {
  expect(await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)).toBeLessThanOrEqual(0)
}

/**
 * A screenshot to look at, not to compare. Taking one costs real time at every width, so it is written only when asked
 * for: `SCREENSHOTS=1 npm run test:visual` puts them in the test's output folder.
 */
export async function lookAt(target: Page | Locator, testInfo: TestInfo, name: string, fullPage = false) {
  if (!process.env.SCREENSHOTS) return
  const path = testInfo.outputPath(`${name}.png`)
  await ('goto' in target ? target.screenshot({ path, fullPage }) : target.screenshot({ path }))
}

/**
 * A control may be drawn small and still be a touch target if the area around it answers for it. Asks the browser what is
 * under a point 22 px above, below, left and right of the control's centre (a 44 px target), and expects the control itself.
 */
export async function expectTapArea(target: Locator, size = 44) {
  const box = await target.boundingBox()
  expect(box).not.toBeNull()
  const half = size / 2 - 1
  const cx = box!.x + box!.width / 2
  const cy = box!.y + box!.height / 2
  const hit = await target.evaluate((el, pts) => pts.map(([x, y]) => {
    const found = document.elementFromPoint(x, y)
    return !!found && (found === el || el.contains(found))
  }), [[cx, cy - half], [cx, cy + half], [cx - Math.min(half, box!.width / 2 - 1), cy], [cx + Math.min(half, box!.width / 2 - 1), cy]] as [number, number][])
  expect(hit, 'the area around the control answers for it').toEqual([true, true, true, true])
}

/**
 * A long list of problems starts folded to its titles; opens every problem card. It is setup, not the thing under test, so it
 * clicks in the page: on a phone the fixed bottom bar can sit over a header Playwright scrolled to, and the click then waits.
 */
export async function openProblemCards(page: Page) {
  await page.locator('[data-test="problem-groups"]').first().waitFor()
  const folded = page.locator('[data-test="problem-groups"] [data-test="card-toggle"][aria-expanded="false"]')
  while ((await folded.count()) > 0) await folded.first().evaluate((el) => (el as HTMLElement).click())
}
