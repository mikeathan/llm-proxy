import { expect, test } from '@playwright/test'
import { expectNoSidewaysScroll, mockAdminApi } from './support/adminApi'

// A stack's Modules tab: layers drawn top to bottom, one tile (a row on a phone) per module with its name, purpose, switch and
// one status line. At both widths nothing scrolls sideways, and on a phone every switch sits in a 44 px target. The data is
// invented; the shape is the server's.
const module = (id: string, title: string, over: Record<string, unknown> = {}) => ({
  id, title, purpose: `${title} does its one job.`, sends: 'lan', step_up: false, enabled: true, needs: [], ...over,
})
const NETWORK = {
  id: 'network', title: 'Network', purpose: '', actions: [], summary: { on: 3, total: 5, stopped: 1, blocked: 1 },
  layers: [
    { id: 'tell', title: 'Tell', purpose: 'Reach you', modules: [module('summary', 'Weekly summary', { enabled: false, sends: 'message', blocked: 'needs Presence check', needs: ['presence'] })] },
    { id: 'outside', title: 'Look outside', purpose: 'What the internet sees', modules: [module('dns', 'Router DNS check', { sends: 'internet', blocked: 'internet access is off in Guardrails' })] },
    { id: 'listen', title: 'Listen', modules: [module('decoys', 'Decoys', { sends: 'listens', step_up: true, health: 'stopped' })] },
    { id: 'discover', title: 'Discover', purpose: 'Who is on the network', modules: [module('presence', 'Presence check', { health: 'working' }), module('names', 'Device names')] },
  ],
}
const AGENTS = {
  id: 'agents', title: 'Agents', purpose: '', actions: [{ id: 'lockdown', title: 'Lock down', purpose: '' }], summary: { on: 2, total: 2, stopped: 0, blocked: 0 },
  layers: [{ id: 'watch', title: 'Watch', modules: [module('agent_activity', 'Agent activity', { sends: 'message', step_up: true }), module('secrets_pass', 'Secrets pass', { sends: 'message', step_up: true })] }],
}

test('the Network Modules tab fits the screen and its switches are touch targets', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: { '/stacks/network': NETWORK } })
  await page.goto('network/modules')
  await expect(page.getByRole('heading', { name: '04 — Tell' })).toBeVisible()
  await expect(page.getByRole('heading', { name: '01 — Discover' })).toBeVisible()
  await expect(page.locator('[data-test="module-summary"] [data-test="module-status"]')).toHaveText('Off · sends a message · needs Presence check')
  await expect(page.locator('[data-test="module-decoys"] [data-test="module-status"]')).toHaveText('Stopped · listens')
  await expect(page.locator('[data-test="stack-summary"]')).toContainText('3 of 5 on')
  await expectNoSidewaysScroll(page)
  if (testInfo.project.name === 'mobile') {
    for (const sw of await page.locator('[data-test="stack-modules"] label:has(input[role="switch"])').all()) {
      expect((await sw.boundingBox())?.height).toBeGreaterThanOrEqual(44)
    }
  }
})

test('the Agents page opens on the activity and has a Modules tab with its two parts', async ({ page }) => {
  await mockAdminApi(page, { answers: { '/stacks/agents': AGENTS } })
  await page.goto('agents')
  await expect(page.getByRole('heading', { name: 'Agents', level: 1 })).toBeVisible()
  await expect(page.locator('[data-test="lockdown"]')).toBeVisible()
  await page.getByRole('link', { name: 'Modules', exact: true }).last().click()
  await expect(page.getByRole('switch', { name: 'Agent activity' })).toBeVisible()
  await expect(page.getByRole('switch', { name: 'Secrets pass' })).toBeVisible()
  await expectNoSidewaysScroll(page)
})
