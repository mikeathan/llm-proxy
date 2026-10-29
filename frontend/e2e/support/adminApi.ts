import { expect, type Locator, type Page, type TestInfo } from '@playwright/test'
import { DEFAULT_CONFIG } from '../../src/composables/models/useConfig'

// What every admin page asks for when it loads, so a spec names only the answers it cares about. Endpoints the pages
// read as lists answer `[]` (a `{}` there throws in the page and hides the section under test); anything unknown `{}`.
const LIST_ENDPOINTS = ['/manifests', '/mcp', '/workspaces', '/profiles', '/profile-schema', '/automations']
const READY = { ready: true, requirements: [], notices: [] }
// Signed in, so no spec sees the login page or the "sign-in is not set up" banner unless it asks for them.
const SIGNED_IN = { required: true, setup_open: false, authenticated: true, elevated: false, encrypted: true }

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
    const json = given
      ? answers[given]
      : path.endsWith('/state')
        ? state
        : path.endsWith('/netaudit/readiness')
          ? READY
          : path.endsWith('/auth/status')
            ? SIGNED_IN
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
