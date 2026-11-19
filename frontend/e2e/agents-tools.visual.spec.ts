import { expect, test } from '@playwright/test'
import { expectNoSidewaysScroll, expectTapArea, lookAt, mockAdminApi } from './support/adminApi'

// Agents · Tools: the MCP servers the agents may use. Empty with the one thing to do, a list that shows every state in words with the next step, the form,
// and the switched-off notice. Nothing scrolls sideways at either width, and on a phone each control is a 44 px target. The data is invented; the shape is
// the server's.
const server = (name: string, over: Record<string, unknown> = {}) => ({
  name, url: `https://${name}.example.com/mcp`, enabled: true, trust: 'ask', auth_kind: 'none', has_secret: false, status: 'ready', server_name: 'fixture',
  server_version: '1.2.3', protocol: '2025-06-18', transport: 'streamable_http', disabled_tools: [],
  tools: [
    { name: 'read_text_file', description: 'Read the complete contents of a file as text.' },
    { name: 'write_file', description: 'Create a new file or overwrite an existing one.' },
    { name: 'delete_everything', description: 'Remove every file below the folder.', disabled: true },
  ],
  ...over,
})
const LIST = {
  enabled: true,
  servers: [
    server('files', { disabled_tools: ['delete_everything'] }),
    server('calendar', { status: 'stopped', tools: [], problem: 'the server wants a sign-in it was not given (401)', problem_code: 'sign_in', auth_kind: 'bearer' }),
    server('notes', { status: 'connecting', tools: [] }),
    server('old-box', { status: 'ready', transport: 'sse', trust: 'allow', auth_kind: 'header', auth_header: 'X-Api-Key', has_secret: true }),
    server('spare', { status: 'off', enabled: false, tools: [] }),
  ],
}

test('with no server the page says what to do and the form fits the screen', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: { '/mcp/servers': { enabled: true, servers: [] } } })
  await page.goto('agents/tools')
  await expect(page.getByRole('heading', { name: 'Agents', level: 1 })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'No MCP servers' })).toBeVisible()
  await lookAt(page, testInfo, 'tools-empty')
  await page.getByRole('button', { name: 'Add a server' }).click()
  await expect(page.locator('[data-test="mcp-form"]')).toBeVisible()
  await page.locator('[data-test="mcp-name"]').fill('files')
  await page.locator('[data-test="mcp-url"]').fill('https://files.example.com/mcp')
  await page.getByRole('radio', { name: 'Bearer token' }).click()
  await expect(page.locator('[data-test="mcp-secret"]')).toHaveAttribute('type', 'password')
  await expect(page.locator('[data-test="mcp-trust-hint"]')).toContainText('waits for your approval')
  await expect(page.locator('[data-test="mcp-save"]')).toBeDisabled()
  await page.locator('[data-test="mcp-secret"]').fill('token')
  await expect(page.locator('[data-test="mcp-save"]')).toBeEnabled()
  await expectNoSidewaysScroll(page)
  await lookAt(page, testInfo, 'tools-form', true)
  if (testInfo.project.name === 'mobile') {
    for (const id of ['mcp-save', 'mcp-test-form', 'mcp-cancel']) {
      const control = page.locator(`[data-test="${id}"]`)
      await control.evaluate((el) => el.scrollIntoView({ block: 'center' })) // clear of the phone's fixed tab bar
      await expectTapArea(control)
    }
  }
})

test('every state is said in words, with the next step, and the tools list opens on demand', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: { '/mcp/servers': LIST } })
  await page.goto('agents/tools')
  await expect(page.locator('[data-test="mcp-server-files"]')).toBeVisible()
  await expect(page.locator('[data-test="mcp-server-files"] [data-test="mcp-status"]')).toHaveText('Connected')
  await expect(page.locator('[data-test="mcp-server-files"] [data-test="mcp-state"]')).toHaveText('over streamable HTTP · 2 tools of 3 · fixture 1.2.3')
  await expect(page.locator('[data-test="mcp-server-calendar"] [data-test="mcp-state"]')).toContainText('Edit the server and give it the token or header it expects.')
  await expect(page.locator('[data-test="mcp-server-notes"] [data-test="mcp-status"]')).toHaveText('Connecting')
  await expect(page.locator('[data-test="mcp-server-old-box"] [data-test="mcp-state"]')).toContainText('the older SSE transport')
  await expect(page.locator('[data-test="mcp-server-spare"] [data-test="mcp-state"]')).toContainText('nothing is sent')
  await page.locator('[data-test="mcp-server-files"] [data-test="mcp-tools-toggle"]').click()
  await expect(page.locator('[data-test="mcp-server-files"] [data-test="mcp-tools"]')).toContainText('read_text_file')
  await expect(page.getByRole('switch', { name: 'Offer delete_everything' })).not.toBeChecked()
  await expectNoSidewaysScroll(page)
  await lookAt(page, testInfo, 'tools-list', true)
  if (testInfo.project.name === 'mobile') {
    for (const id of ['mcp-edit', 'mcp-remove', 'mcp-test', 'mcp-tools-toggle']) {
      const control = page.locator(`[data-test="mcp-server-files"] [data-test="${id}"]`)
      await control.evaluate((el) => el.scrollIntoView({ block: 'center' })) // clear of the phone's fixed tab bar
      await test.step(id, () => expectTapArea(control))
    }
    for (const sw of await page.locator('[data-test="mcp-server-files"] label:has(input[role="switch"])').all()) {
      expect((await sw.boundingBox())?.height).toBeGreaterThanOrEqual(44)
    }
  }
})

test('editing keeps the stored value out of the form and holds a new address to a new value', async ({ page }) => {
  await mockAdminApi(page, { answers: { '/mcp/servers': LIST } })
  await page.goto('agents/tools')
  await page.locator('[data-test="mcp-server-old-box"] [data-test="mcp-edit"]').click()
  await expect(page.locator('[data-test="mcp-secret"]')).toHaveValue('')
  await expect(page.locator('[data-test="mcp-secret"]')).toHaveAttribute('placeholder', /leave empty to keep/)
  await expect(page.locator('[data-test="mcp-name"]')).toBeDisabled()
  await expect(page.locator('[data-test="mcp-save"]')).toBeEnabled()
  await page.locator('[data-test="mcp-url"]').fill('https://elsewhere.example.com/mcp')
  await expect(page.locator('[data-test="mcp-save"]')).toBeDisabled()
  await expect(page.getByText('never sent to another address')).toBeVisible()
  await expectNoSidewaysScroll(page)
})

test('a switched-off module says so and links to where it is switched on', async ({ page }) => {
  await mockAdminApi(page, { answers: { '/mcp/servers': { ...LIST, enabled: false } } })
  await page.goto('agents/tools')
  const off = page.locator('[data-test="mcp-off"]')
  await expect(off).toContainText('MCP servers are switched off')
  await off.getByRole('link', { name: 'Switch it on the Modules tab' }).click()
  await expect(page).toHaveURL(/agents\/modules$/)
})

test('an old Settings link to MCP servers lands on the Tools tab', async ({ page }) => {
  await mockAdminApi(page, { answers: { '/mcp/servers': LIST } })
  await page.goto('settings/mcp')
  await expect(page).toHaveURL(/agents\/tools$/)
  await expect(page.locator('[data-test="mcp-server-files"]')).toBeVisible()
})
