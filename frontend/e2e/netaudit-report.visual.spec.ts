import { expect, test, type Page } from '@playwright/test'
import { DEFAULT_CONFIG } from '../src/composables/models/useConfig'

// The audit's report with problems among plain open ports, in Simple and Advanced at both widths: nothing may scroll
// sideways, and the screenshots are for looking at how clearly a problem stands out.
const finding = (over: Record<string, unknown>) => ({
  key: 'k', device: '192.0.2.10', port: 22, proto: 'tcp', service: 'ssh', check: 'open_port', severity: 'info', new: false, changed: false, accepted: false, ...over,
})
const report = {
  scanned: true, scanned_at: '2026-10-26T13:38:00Z', engine: 'nmap', devices_up: 8, unscanned: ['192.0.2.107'], resolved: [],
  findings: [
    ...[1, 10, 43, 60, 63, 241].flatMap((n, i) => [22, 80, 443, 8080].slice(0, (i % 4) + 1).map((port) => finding({ key: `p${n}-${port}`, device: `192.0.2.${n}`, port, service: port === 22 ? 'ssh' : 'http' }))),
    finding({ key: 'cert', device: '192.0.2.43', port: 8009, service: 'ajp13 (guess)', check: 'tls_cert_expiring', severity: 'low', evidence: 'The certificate expires on 2026-11-20', new: true }),
    finding({ key: 'smb', device: '192.0.2.10', port: 445, service: 'microsoft-ds', check: 'smb_v1', severity: 'high', evidence: 'SMBv1, an old and unsafe file-sharing version, is enabled', new: true }),
    finding({ key: 'redis', device: '192.0.2.60', port: 6379, service: 'redis', check: 'redis_no_auth', severity: 'critical', evidence: 'Redis answers without a password' }),
  ],
}
const automation = {
  id: 'ws/network-audit', workspace: 'ws', name: 'network-audit', task_file: 'network-audit.md', strategy: 'persistent', trigger: 'cron',
  trigger_value: '0 2 * * *', profile: 'security-auditor', network_grant: 'lan', scan: { depth: 'thorough', checks: true }, history: [{ id: 'r1', timestamp: '2026-10-26T13:30:00Z', error: '', output: '', duration_ms: 1000, model: '' }],
}

async function open(page: Page, view: 'simple' | 'advanced') {
  await page.addInitScript((v) => localStorage.setItem('admin-ui:netaudit-view', JSON.stringify({ version: 1, data: v })), view)
  await page.route('**/admin/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    const json = path.endsWith('/state')
      ? { config: structuredClone(DEFAULT_CONFIG), models: [], available_models: [] }
      : path.endsWith('/netaudit/findings')
        ? report
        : path.endsWith('/netaudit/readiness')
          ? { ready: true, requirements: [] }
          : path.endsWith('/automations')
            ? [automation]
            : path.endsWith('/workspaces')
              ? [{ id: 'ws' }]
              : path.endsWith('/manifests') || path.endsWith('/mcp') || path.endsWith('/profiles') || path.endsWith('/profile-schema')
                ? []
                : {}
    await route.fulfill({ json })
  })
  await page.goto('automations/ws%2Fnetwork-audit')
  await expect(page.getByRole('heading', { name: 'Latest network scan' })).toBeVisible()
}

for (const view of ['simple', 'advanced'] as const) {
  test(`audit report with problems (${view}) fits the screen`, async ({ page }, testInfo) => {
    await open(page, view)
    expect(await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)).toBeLessThanOrEqual(0)
    await page.getByRole('heading', { name: 'Latest network scan' }).scrollIntoViewIfNeeded()
    await page.screenshot({ path: testInfo.outputPath(`report-${view}.png`), fullPage: true })
  })
}
