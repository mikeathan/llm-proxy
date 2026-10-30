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
// Signed in, so no spec sees the login page or the "sign-in is not set up" banner unless it asks for them.
const SIGNED_IN = { set_up: true, setup_open: false, authenticated: true, elevated: false, encrypted: true }

const DEFAULT_ANSWERS: Record<string, unknown> = {
  '/netaudit/readiness': READY,
  '/auth/status': SIGNED_IN,
  '/netaudit/devices': DEVICES,
  '/netaudit/presence': PRESENCE,
  '/netaudit/decoys': DECOYS,
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
