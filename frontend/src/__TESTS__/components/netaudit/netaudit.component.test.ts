import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { FindingsReport as Report, ScanFinding, ScanScope, ScanSettings, VulnDataStatus } from '../../../types/netaudit'

const service = vi.hoisted(() => ({ checkTargets: vi.fn(), readiness: vi.fn(), scope: vi.fn(), updateScope: vi.fn(), findings: vi.fn(), accept: vi.fn(), revoke: vi.fn(), vulnData: vi.fn(), startVulnDownload: vi.fn(), cancelVulnDownload: vi.fn(), markFixed: vi.fn(), unmarkFixed: vi.fn(), clearVulnData: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))
const confirmMock = vi.hoisted(() => vi.fn())
vi.mock('../../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm: confirmMock }) }))
const dispatcher = vi.hoisted(() => ({ listAutomations: vi.fn(), listWorkspaces: vi.fn(), triggerAutomation: vi.fn() }))
vi.mock('../../../services/automation/dispatcherService', () => ({ DispatcherService: dispatcher }))
const push = vi.hoisted(() => vi.fn())
const routeState = vi.hoisted(() => ({ hash: '' }))
vi.mock('vue-router', async (importOriginal) => ({ ...(await importOriginal<typeof import('vue-router')>()), useRouter: () => ({ push }), useRoute: () => routeState }))

import VulnDataCard from '../../../components/netaudit/VulnDataCard.vue'
import ScanSettingsFields from '../../../components/netaudit/ScanSettingsFields.vue'
import FindingsReport from '../../../components/netaudit/FindingsReport.vue'
import ReadinessBanner from '../../../components/netaudit/ReadinessBanner.vue'
import NetworkScanSettings from '../../../components/settings/NetworkScanSettings.vue'
import ScanSummary from '../../../components/netaudit/ScanSummary.vue'
import AuditSetupCheck from '../../../components/netaudit/AuditSetupCheck.vue'
import FailureExplanation from '../../../components/netaudit/FailureExplanation.vue'
import NetauditHelp from '../../../components/netaudit/NetauditHelp.vue'
import { useNetauditViewMode } from '../../../composables/netaudit/useNetauditViewMode'
import { useNetauditHelp } from '../../../composables/netaudit/useNetauditHelp'
import { useNetauditReadiness } from '../../../composables/netaudit/useNetauditReadiness'
import { useVulnData } from '../../../composables/netaudit/useVulnData'

const finding = (over: Partial<ScanFinding>): ScanFinding => ({
  key: 'k', device: '192.168.1.5', port: 22, proto: 'tcp', service: 'ssh', check: 'open_port', severity: 'info', new: false, changed: false, accepted: false, ...over,
})
const report: Report = {
  scanned: true, scanned_at: '2026-10-25T01:00:00Z', engine: 'nmap',
  findings: [
    finding({ key: 'a', port: 22, new: true }),
    finding({ key: 'b', device: '192.168.1.9', port: 23, service: 'telnet', new: true }),
    finding({ key: 'c', port: 443, service: 'https' }),
    finding({ key: 'e', port: 8022, service: 'ssh', version: 'OpenSSH 9.7p1', changed: true, previous_version: 'OpenSSH 9.6p1' }),
    finding({ key: 'd', port: 445, accepted: true, accepted_note: 'my NAS', accepted_until: '2027-01-01T00:00:00Z' }),
  ],
  resolved: [{ device: '192.168.1.7', port: 80, proto: 'tcp' }],
  unscanned: ['192.168.1.34'], devices_up: 10,
}
const NEVER = '0001-01-01T00:00:00Z'
const emptySource = { imported_at: NEVER, cves: 0, entries: 0 }
const vulnStatus = (over: Partial<VulnDataStatus> = {}): VulnDataStatus => ({ state: 'idle', files_done: 0, files_total: 6, nvd: emptySource, kev: emptySource, ...over })
const recent = new Date(Date.now() - 3 * 86_400_000).toISOString()
const vulnHeld = (): VulnDataStatus =>
  vulnStatus({ state: 'done', files_done: 6, nvd: { imported_at: recent, cves: 160000, entries: 900000, years: [2022, 2023, 2024, 2025, 2026] }, kev: { imported_at: recent, cves: 1758, entries: 0 } })
const lan = { interface: 'en0', cidr: '192.168.1.0/24', devices: 254 }
const emptyScope: ScanScope = { ranges: [], allow_unconfined: false, device_timeout_minutes: 0, suggestions: [lan] }

beforeEach(() => {
  for (const fn of Object.values(service)) fn.mockReset()
  push.mockReset()
  dispatcher.listAutomations.mockReset().mockResolvedValue([])
  dispatcher.listWorkspaces.mockReset().mockResolvedValue([{ id: 'ws' }])
  dispatcher.triggerAutomation.mockReset().mockResolvedValue({ status: 'started' })
  routeState.hash = ''
  service.readiness.mockResolvedValue({ ready: true, requirements: [], notices: [] })
  service.vulnData.mockResolvedValue(vulnStatus())
  // The download's state is shared by every screen, so each test starts from none.
  Object.assign(useVulnData(), {}).status.value = null
  useVulnData().error.value = ''
  useVulnData().actionError.value = ''
  useNetauditViewMode().set('simple')
  useNetauditHelp().hide()
})

describe('FindingsReport', () => {
  async function mountReport() {
    service.findings.mockResolvedValue(report)
    const w = mount(FindingsReport, { props: { workspaceId: 'ws', automation: 'nightly' }, attachTo: document.body })
    await flushPromises()
    return w
  }

  it('says in plain words what needs a look, and offers one decision for each', async () => {
    const w = await mountReport()
    expect(w.get('[data-test="summary"]').text()).toBe('3 things need a look')
    expect(w.text()).toContain('ssh (port 8022) on 192.168.1.5 changed to OpenSSH 9.7p1')
    expect(w.text()).toContain('Remote login (port 22) on 192.168.1.5')
    expect(w.text()).toContain('It sends everything unencrypted.')
    expect(w.text()).not.toContain('22/tcp')
    expect(w.text()).toContain('Gone since last time')
  })

  it('shows the Advanced report like the board: counts, devices, versions and states', async () => {
    useNetauditViewMode().set('advanced')
    const w = await mountReport()
    expect(w.get('[data-test="scan-meta"]').text()).toContain('nmap · 10 devices up · 5 open ports')
    const tiles = w.findAll('button[aria-pressed]').map((b) => b.text())
    expect(tiles).toEqual(['0problems found by checks', '2new since the last scan', '1changed', '1resolved', '1accepted (hidden from the assistant)'])
    expect(w.text()).toContain('1 device was not scanned to completion')
    expect(w.text()).toContain('192.168.1.34')
    expect(w.findAll('section[aria-label="192.168.1.5"]')).toHaveLength(1)
    expect(w.get('[data-key="e"]').text()).toContain('OpenSSH 9.6p1')
    expect(w.get('[data-key="e"] s').text()).toBe('OpenSSH 9.6p1')
    expect(w.get('[data-key="e"]').text()).toContain('changed')
    expect(w.get('[data-key="d"]').text()).toContain('Accepted until')
    expect(w.get('[data-key="d"]').text()).toContain('my NAS')
    expect(w.get('[data-key="d"]').text()).toContain('Undo')
    expect(w.get('[data-key="a"]').text()).toContain('Accept…')
  })

  it('filters the list from a tile and shows what resolved', async () => {
    useNetauditViewMode().set('advanced')
    const w = await mountReport()
    await w.findAll('button[aria-pressed]').find((b) => b.text().includes('changed'))!.trigger('click')
    expect(w.findAll('li[data-key]').map((li) => li.attributes('data-key'))).toEqual(['e'])
    await w.findAll('button[aria-pressed]').find((b) => b.text().includes('resolved'))!.trigger('click')
    expect(w.text()).toContain('is no longer there')
    expect(w.text()).toContain('192.168.1.7')
  })

  it('only says earlier findings are kept when there are some', async () => {
    useNetauditViewMode().set('advanced')
    const w = await mountReport()
    expect(w.text()).toContain('ran out of time, even after a second try')
    expect(w.text()).not.toContain('earlier findings are kept')
    service.findings.mockResolvedValue({ ...report, unscanned: ['192.168.1.5'] })
    const again = mount(FindingsReport, { props: { workspaceId: 'ws', automation: 'nightly' } })
    await flushPromises()
    expect(again.text()).toContain('Their earlier findings are kept.')
  })

  it('takes you to the time setting in Advanced from "Allow more time"', async () => {
    const w = await mountReport()
    await w.findAll('button').find((b) => b.text() === 'Allow more time')!.trigger('click')
    expect(useNetauditViewMode().mode.value).toBe('advanced')
    expect(push).toHaveBeenCalledWith({ name: expect.any(String), params: { section: 'network-scan' }, hash: '#scan-time' })
  })

  it('reads the report again when a run starts or finishes, in either view', async () => {
    service.findings.mockResolvedValue({ scanned: false, findings: [], resolved: [], unscanned: [], devices_up: 0 })
    const w = mount(FindingsReport, { props: { workspaceId: 'ws', automation: 'nightly', refreshKey: 'true|run1' } })
    await flushPromises()
    expect(w.text()).toContain('Not scanned yet')
    service.findings.mockResolvedValue(report)
    await w.setProps({ refreshKey: 'false|run2' })
    await flushPromises()
    expect(service.findings).toHaveBeenCalledTimes(2)
    expect(w.text()).toContain('3 things need a look')
    useNetauditViewMode().set('advanced')
    await flushPromises()
    expect(w.find('[data-test="findings-advanced"]').exists()).toBe(true)
  })

  it('takes an acceptance back with Undo in the Advanced view', async () => {
    useNetauditViewMode().set('advanced')
    service.revoke.mockResolvedValue(undefined)
    const w = await mountReport()
    await w.get('[data-key="d"] button').trigger('click')
    expect(service.revoke).toHaveBeenCalledWith('ws', 'd')
  })

  it('accepts a finding through the dialog and reloads', async () => {
    service.accept.mockResolvedValue({})
    const w = await mountReport()
    await w.findAll('button').find((b) => b.text() === 'This is fine')!.trigger('click')
    expect(document.body.textContent).toContain('For how long')
    const dialogButtons = Array.from(document.body.querySelectorAll('[role="dialog"] button'))
    ;(dialogButtons.find((b) => b.textContent?.trim() === 'This is fine') as HTMLElement).click()
    await flushPromises()
    expect(service.accept).toHaveBeenCalledWith({ workspace_id: 'ws', automation: 'nightly', key: 'a', note: '', days: 90 })
    expect(service.findings).toHaveBeenCalledTimes(2)
    w.unmount()
  })

  it('takes an acceptance back', async () => {
    service.revoke.mockResolvedValue(undefined)
    const w = await mountReport()
    await w.findAll('button').find((b) => b.text() === 'Take back')!.trigger('click')
    expect(service.revoke).toHaveBeenCalledWith('ws', 'd')
  })

  it('says so when nothing has been scanned yet', async () => {
    service.findings.mockResolvedValue({ scanned: false, findings: [], resolved: [], unscanned: [], devices_up: 0 })
    const w = mount(FindingsReport, { props: { workspaceId: 'ws', automation: 'nightly' } })
    await flushPromises()
    expect(w.text()).toContain('Not scanned yet')
  })

  it('shows why the report could not be read, with a retry', async () => {
    service.findings.mockRejectedValue(new Error('boom'))
    const w = mount(FindingsReport, { props: { workspaceId: 'ws', automation: 'nightly' } })
    await flushPromises()
    expect(w.text()).toContain('boom')
    expect(w.text()).toContain('Retry')
  })
})

describe('FindingsReport with checks', () => {
  const withChecks: Report = {
    scanned: true, scanned_at: '2026-10-26T02:00:00Z', engine: 'nmap', unscanned: [], devices_up: 3, resolved: [],
    findings: [
      finding({ key: 'p', device: '192.0.2.10', port: 445, service: 'microsoft-ds', new: true }),
      finding({ key: 'smb', device: '192.0.2.10', port: 445, check: 'smb_v1', severity: 'high', evidence: 'SMBv1, an old and unsafe file-sharing version, is enabled', new: true }),
      finding({ key: 'p2', device: '192.0.2.20', port: 6379, service: 'redis', new: true }),
      finding({ key: 'redis', device: '192.0.2.20', port: 6379, check: 'redis_no_auth', severity: 'critical', evidence: 'Redis answers without a password', new: true }),
    ],
  }
  async function mountChecks() {
    service.findings.mockResolvedValue(withChecks)
    const w = mount(FindingsReport, { props: { workspaceId: 'ws', automation: 'audit' }, attachTo: document.body })
    await flushPromises()
    return w
  }

  it('Simple: the worst problem is listed first, in plain words, and counts include the checks', async () => {
    const w = await mountChecks()
    expect(w.get('[data-test="summary"]').text()).toBe('4 things need a look')
    const groups = w.findAll('article[data-check]')
    expect(groups.map((g) => g.attributes('data-check'))).toEqual(['redis_no_auth', 'smb_v1'])
    expect(groups[0]!.text()).toContain('Redis answers without a password')
    expect(groups[0]!.text()).toContain('on 192.0.2.20')
    expect(groups[1]!.text()).toContain('SMBv1, an old and unsafe file-sharing version, is enabled')
    expect(w.text()).toContain('2 services on 2 devices, 2 problems found by checks')
    expect(w.text()).not.toContain('smb_v1')
  })

  it('Simple: a problem stands out by colour and tag, and stays listed after the scan it was found in', async () => {
    const old: Report = {
      ...withChecks,
      findings: [
        finding({ key: 'p', device: '192.0.2.10', port: 445, service: 'microsoft-ds' }),
        finding({ key: 'smb', device: '192.0.2.10', port: 445, check: 'smb_v1', severity: 'high', evidence: 'SMBv1 is enabled' }),
        finding({ key: 'cert', device: '192.0.2.43', port: 8009, check: 'tls_cert_expiring', severity: 'low', evidence: 'The certificate expires on 2026-12-01' }),
      ],
    }
    service.findings.mockResolvedValue(old)
    const w = mount(FindingsReport, { props: { workspaceId: 'ws', automation: 'audit' } })
    await flushPromises()
    // Nothing is new, yet the two problems are still there to deal with; the plain open port is not listed.
    expect(w.get('[data-test="summary"]').text()).toBe('2 things need a look')
    expect(w.get('[data-test="problems"]').text()).toBe('2 problems found by checks')
    const rows = w.findAll('article[data-severity]')
    expect(rows.map((r) => r.attributes('data-severity'))).toEqual(['high', 'low'])
    expect(rows[0]!.classes()).toEqual(expect.arrayContaining(['border-l-4', 'border-state-error']))
    expect(rows[1]!.classes()).toEqual(expect.arrayContaining(['border-l-4', 'border-state-running']))
    expect(rows[1]!.text()).toContain('Problem · low')
    expect(rows[1]!.get('[data-test="problem-title"]').text()).toContain('The certificate expires on 2026-12-01')
  })

  it('Advanced: the Problems tile and filter list every unaccepted problem, new or not', async () => {
    useNetauditViewMode().set('advanced')
    const w = await mountChecks()
    await w.findAll('button[aria-pressed]').find((b) => b.text().includes('problems found by checks'))!.trigger('click')
    expect(w.findAll('[data-test="problem"]').map((li) => li.attributes('data-key'))).toEqual(['redis', 'smb'])
    const group = w.get('article[data-check="redis_no_auth"]')
    expect(group.classes()).toEqual(expect.arrayContaining(['border-l-4', 'border-state-error']))
    expect(group.text()).toContain('Problem · critical')
    // The problems are listed once, in the Problems block; the filter shows no device list under them.
    expect(w.findAll('[data-test="device"]')).toHaveLength(0)
  })

  it('Simple: the accept dialog names the check, not a port', async () => {
    const w = await mountChecks()
    await w.findAll('button').find((b) => b.text() === 'This is fine')!.trigger('click')
    expect(document.body.textContent).toContain('192.0.2.20: Redis answers without a password will stop being listed')
    w.unmount()
  })

  it('Advanced: a check row shows its severity and the finding, and the counts keep ports and problems apart', async () => {
    useNetauditViewMode().set('advanced')
    const w = await mountChecks()
    expect(w.get('[data-test="scan-meta"]').text()).toContain('2 open ports · 2 problems found by checks')
    expect(w.get('article[data-check="smb_v1"]').text()).toContain('high')
    expect(w.get('[data-key="smb"] [data-test="check"]').text()).toContain('SMBv1, an old and unsafe file-sharing version, is enabled')
    expect(w.get('section[aria-label="192.0.2.10"]').text()).toContain('1 open port, 1 problem')
    // The device list shows the port, not the problem again.
    expect(w.get('section[aria-label="192.0.2.10"]').find('[data-key="smb"]').exists()).toBe(false)
    expect(w.get('[data-key="p"]').text()).toContain('microsoft-ds')
  })
})

describe('FindingsReport grouped problems', () => {
  const FIX = 'Update the software to the vendor current release.'
  const ssh = (key: string, device: string) =>
    finding({ key, device, port: 22, check: 'known_vulns', check_title: 'Software with known flaws', severity: 'low', evidence: 'May be affected by 29 known flaws', fix: FIX, new: true })
  const grouped: Report = {
    scanned: true, scanned_at: '2026-10-28T02:00:00Z', engine: 'nmap', unscanned: [], devices_up: 3, resolved: [],
    findings: [
      finding({ key: 'p1', device: '192.0.2.10', port: 22, version: 'OpenSSH 9.6p1', new: true }),
      ssh('v1', '192.0.2.10'), ssh('v2', '192.0.2.20'), ssh('v3', '192.0.2.30'),
    ],
  }
  async function mountGrouped(over: Partial<Report> = {}) {
    service.findings.mockResolvedValue({ ...grouped, ...over })
    const w = mount(FindingsReport, { props: { workspaceId: 'ws', automation: 'audit' } })
    await flushPromises()
    return w
  }

  it('shows one problem found in three places as one card: the title and the fix once, each place with its own decision', async () => {
    const w = await mountGrouped()
    const cards = w.findAll('article[data-check]')
    expect(cards).toHaveLength(1)
    expect(cards[0]!.get('[data-test="problem-title"]').text()).toBe('Software with known flaws')
    expect(cards[0]!.text()).toContain('3 places')
    expect(w.findAll('[data-test="fix"]')).toHaveLength(1)
    expect(cards[0]!.findAll('[data-test="problem"]').map((r) => r.attributes('data-key'))).toEqual(['v1', 'v2', 'v3'])
    expect(cards[0]!.findAll('button').filter((b) => b.text() === 'This is fine')).toHaveLength(3)
  })

  it('accepts one place of a group without touching the others', async () => {
    const w = await mountGrouped()
    await w.get('[data-key="v2"]').findAll('button').find((b) => b.text() === 'This is fine')!.trigger('click')
    expect(document.body.textContent).toContain('192.0.2.20')
    w.unmount()
  })

  it('treats the first scan as a baseline: no port is new, the note says so, and what was found stays one tap away', async () => {
    const w = await mountGrouped({ first_scan: true, findings: [finding({ key: 'p1', device: '192.0.2.10', port: 22, version: 'OpenSSH 9.6p1', new: true }), finding({ key: 'p2', device: '192.0.2.10', port: 80, service: 'http', new: true })] })
    expect(w.get('[data-test="summary"]').text()).toBe('Nothing needs a look')
    expect(w.get('[data-test="baseline-note"]').text()).toContain('first scan')
    expect(w.get('[data-test="inventory"]').text()).toContain('What was found')
    expect(w.findAll('button').filter((b) => b.text() === 'This is fine')).toHaveLength(0)
  })

  it('still lists a problem on the first scan: a baseline hides new ports, not problems', async () => {
    const w = await mountGrouped({ first_scan: true })
    expect(w.get('[data-test="summary"]').text()).toBe('3 things need a look')
  })

  it('Advanced: a device with nothing to look at is folded behind "Show ports", and opens on demand', async () => {
    useNetauditViewMode().set('advanced')
    const w = await mountGrouped({
      first_scan: true,
      findings: [finding({ key: 'p1', device: '192.0.2.10', port: 22, version: 'OpenSSH 9.6p1' }), finding({ key: 'p2', device: '192.0.2.10', port: 80, service: 'http', version: 'nginx 1.2' })],
    })
    const section = w.get('section[aria-label="192.0.2.10"]')
    expect(section.find('[data-key="p1"]').exists()).toBe(false)
    expect(section.text()).toContain('2 open ports')
    await section.get('[data-test="toggle-device"]').trigger('click')
    expect(section.find('[data-key="p1"]').exists()).toBe(true)
    expect(section.get('[data-test="toggle-device"]').text()).toBe('Hide ports')
  })

  it('Advanced: ports the scan could not identify fold into one line, and can be shown', async () => {
    useNetauditViewMode().set('advanced')
    const w = await mountGrouped({
      findings: [
        finding({ key: 'u1', device: '192.0.2.40', port: 8443, service: 'https-alt (guess)', new: true }),
        finding({ key: 'u2', device: '192.0.2.40', port: 49152, service: 'unknown (guess)', new: true }),
        finding({ key: 'ok', device: '192.0.2.40', port: 22, version: 'OpenSSH 9.6p1', new: true }),
      ],
    })
    const section = w.get('section[aria-label="192.0.2.40"]')
    expect(section.find('[data-key="u1"]').exists()).toBe(false)
    expect(section.get('[data-test="low-detail"]').text()).toContain('2 ports the scan could not identify: 8443, 49152')
    expect(section.find('[data-key="ok"]').exists()).toBe(true)
    await section.get('[data-test="low-detail"] button').trigger('click')
    expect(section.find('[data-key="u1"]').exists()).toBe(true)
  })
})

describe('FindingsReport known vulnerabilities', () => {
  const vulns: Report = {
    scanned: true, scanned_at: '2026-10-28T02:00:00Z', engine: 'nmap', unscanned: [], devices_up: 1, resolved: [],
    findings: [
      finding({
        key: 'v', device: '192.0.2.5', port: 22, check: 'known_vulns', severity: 'low', service: 'ssh', version: 'OpenSSH 9.6p1 Ubuntu 3ubuntu13.14',
        evidence: 'The software on this port may be affected by 3 known flaws: CVE-2024-6387, CVE-2023-51767, CVE-2025-26465.', new: true,
        cves: ['CVE-2023-51767', 'CVE-2024-6387', 'CVE-2025-26465'], exploited: ['CVE-2023-51767'], cve_scores: { 'CVE-2023-51767': 7, 'CVE-2024-6387': 8.1, 'CVE-2025-26465': 6.8 },
      }),
    ],
  }
  async function mountVulns() {
    useNetauditViewMode().set('advanced')
    service.findings.mockResolvedValue(vulns)
    const w = mount(FindingsReport, { props: { workspaceId: 'ws', automation: 'audit' }, attachTo: document.body })
    await flushPromises()
    return w
  }

  it('Advanced: names the software that was compared, so a match can be judged', async () => {
    const w = await mountVulns()
    const row = w.get('[data-key="v"]')
    expect(row.get('[data-test="vuln-software"]').text()).toBe('Compared: ssh · OpenSSH 9.6p1 Ubuntu 3ubuntu13.14')
  })

  it('Advanced: lists every flaw, exploited first and then the most serious, each linking to its NVD page', async () => {
    const w = await mountVulns()
    const row = w.get('[data-key="v"]')
    expect(row.get('[data-test="vuln-flaws"] button').attributes('aria-expanded')).toBe('false')
    await row.get('[data-test="vuln-flaws"] button').trigger('click')
    expect(row.get('[data-test="vuln-flaws"] button').attributes('aria-expanded')).toBe('true')
    const flaws = row.findAll('[data-test="vuln-flaw"]')
    expect(flaws.map((f) => f.get('a').text())).toEqual(['CVE-2023-51767', 'CVE-2024-6387', 'CVE-2025-26465'])
    expect(flaws[0]!.text()).toContain('exploited')
    expect(flaws[1]!.text()).toContain('8.1')
    const link = flaws[1]!.get('a')
    expect(link.attributes('href')).toBe('https://nvd.nist.gov/vuln/detail/CVE-2024-6387')
    expect(link.attributes('rel')).toContain('noopener')
    expect(link.attributes('target')).toBe('_blank')
  })

  it('Simple: keeps the one plain line, with no list', async () => {
    useNetauditViewMode().set('simple')
    service.findings.mockResolvedValue(vulns)
    const w = mount(FindingsReport, { props: { workspaceId: 'ws', automation: 'audit' }, attachTo: document.body })
    await flushPromises()
    expect(w.find('[data-test="vuln-flaws"]').exists()).toBe(false)
  })
})

describe('FindingsReport fix guides', () => {
  const guide = {
    steps: ['Find which devices still need SMBv1.', 'Require SMB2 or newer, then restart the service.'],
    commands: [{ platform: 'Linux', command: 'sudo systemctl restart smbd' }],
    risk: 'Devices that only speak SMBv1 lose access.',
    undo: 'Put the saved smb.conf.bak back.',
  }
  const smb = (over: Partial<ScanFinding> = {}) => finding({ key: 'smb', device: '192.0.2.10', port: 445, check: 'smb_v1', severity: 'high', evidence: 'SMBv1, an old and unsafe file-sharing version, is enabled', fix: 'Turn off SMBv1.', guide, new: true, ...over })
  const port = finding({ key: 'p', device: '192.0.2.10', port: 22, service: 'ssh' })
  const report = (...findings: ScanFinding[]): Report => ({ scanned: true, scanned_at: '2026-10-28T12:00:00Z', engine: 'nmap', unscanned: [], devices_up: 1, resolved: [], findings })
  async function mountGuide(r: Report, view: 'simple' | 'advanced' = 'simple') {
    useNetauditViewMode().set(view)
    service.findings.mockResolvedValue(r)
    const w = mount(FindingsReport, { props: { workspaceId: 'ws', automation: 'audit' }, attachTo: document.body })
    await flushPromises()
    return w
  }
  const toggle = (w: Awaited<ReturnType<typeof mountGuide>>) => w.get('[data-test="fix-guide"] button[aria-expanded]')
  const named = (w: Awaited<ReturnType<typeof mountGuide>>, text: string) => w.findAll('button').find((b) => b.text() === text)

  it('Simple: the steps, the risk and the way back are one tap away, without commands', async () => {
    const w = await mountGuide(report(smb(), port))
    expect(toggle(w).attributes('aria-expanded')).toBe('false')
    await toggle(w).trigger('click')
    const body = w.get('[data-test="fix-guide"]').text()
    expect(body).toContain('Require SMB2 or newer, then restart the service.')
    expect(body).toContain('Devices that only speak SMBv1 lose access.')
    expect(body).toContain('Put the saved smb.conf.bak back.')
    expect(body).not.toContain('sudo systemctl restart smbd')
    expect(w.findAll('[data-test="fix-guide"]')).toHaveLength(1)
  })

  it('Advanced: adds the commands to copy, named by the system they are for', async () => {
    const w = await mountGuide(report(smb(), port), 'advanced')
    await toggle(w).trigger('click')
    const commands = w.findAll('[data-test="fix-command"]')
    expect(commands).toHaveLength(1)
    expect(commands[0]!.text()).toContain('Linux')
    expect(commands[0]!.text()).toContain('sudo systemctl restart smbd')
    expect(commands[0]!.find('button[aria-label]').exists()).toBe(true)
  })

  it('"I\'ve done this" tells the server and reads the report again', async () => {
    service.markFixed.mockResolvedValue({ key: 'smb' })
    const w = await mountGuide(report(smb()))
    service.findings.mockClear()
    await named(w, "I've done this")!.trigger('click')
    await flushPromises()
    expect(service.markFixed).toHaveBeenCalledWith({ workspace_id: 'ws', automation: 'audit', key: 'smb', note: '' })
    expect(service.findings).toHaveBeenCalled()
  })

  it('says it is waiting for the next scan after a fix was marked, and lets the operator take that back', async () => {
    service.unmarkFixed.mockResolvedValue(undefined)
    const w = await mountGuide(report(smb({ fix_state: 'marked', fix_marked_at: '2026-10-28T13:00:00Z' })))
    expect(w.get('[data-test="fix-state"]').text()).toContain('You marked this as fixed')
    expect(w.get('[data-test="fix-state"]').text()).toContain('next scan')
    expect(named(w, "I've done this")).toBeUndefined()
    await named(w, 'Take back')!.trigger('click')
    await flushPromises()
    expect(service.unmarkFixed).toHaveBeenCalledWith('ws', 'smb')
  })

  it('says plainly when a scan since still shows the problem, and offers the one-device rescan', async () => {
    const w = await mountGuide(report(smb({ fix_state: 'still_present', fix_marked_at: '2026-10-28T11:00:00Z' })))
    const state = w.get('[data-test="fix-state"]')
    expect(state.text()).toContain('Still showing after you marked it fixed')
    await named(w, 'Scan this device again')!.trigger('click')
    await flushPromises()
    expect(dispatcher.triggerAutomation).toHaveBeenCalledWith('ws', 'audit', undefined, '192.0.2.10')
  })

  it('offers nothing to fix on an open port', async () => {
    const w = await mountGuide(report(port))
    expect(w.find('[data-test="fix-guide"]').exists()).toBe(false)
    expect(named(w, "I've done this")).toBeUndefined()
  })

  it('shows why marking failed and keeps the screen as it was', async () => {
    service.markFixed.mockRejectedValue(new Error('no such finding in the latest scan'))
    const w = await mountGuide(report(smb()))
    await named(w, "I've done this")!.trigger('click')
    await flushPromises()
    expect(w.get('[role="alert"]').text()).toContain('no such finding')
  })

  it.each(['simple', 'advanced'] as const)('%s: a fixed finding the scan confirmed gone says so', async (view) => {
    const w = await mountGuide({ ...report(port), resolved: [{ device: '192.0.2.10', port: 445, proto: 'tcp', check: 'smb_v1', evidence: 'SMBv1 is enabled', fixed_at: '2026-10-27T09:00:00Z' }] }, view)
    if (view === 'advanced') await w.findAll('button[aria-pressed]').find((b) => b.text().includes('resolved'))!.trigger('click')
    expect(w.get('[data-test="fixed-confirmed"]').text()).toContain('you marked it on')
    expect(w.get('[data-test="fixed-confirmed"]').text()).toContain('the scan confirms it')
  })
})

describe('FindingsReport fixes and rescans', () => {
  const FIX = 'Turn off SMBv1 on the device or server.'
  const scanned: Report = {
    scanned: true, scanned_at: '2026-10-26T02:00:00Z', engine: 'nmap', devices_up: 3, resolved: [], unscanned: ['192.0.2.30'],
    findings: [
      finding({ key: 'smb', device: '192.0.2.10', port: 445, check: 'smb_v1', severity: 'high', evidence: 'SMBv1 is enabled', fix: FIX, new: true }),
      finding({ key: 'p', device: '192.0.2.10', port: 445, service: 'microsoft-ds', new: true }),
    ],
  }
  async function mountScanned() {
    service.findings.mockResolvedValue(scanned)
    const w = mount(FindingsReport, { props: { workspaceId: 'ws', automation: 'audit' } })
    await flushPromises()
    return w
  }
  const button = (w: Awaited<ReturnType<typeof mountScanned>>, text: string) => w.findAll('button').find((b) => b.text() === text)!

  it('shows the suggested fix beside a problem in both views, and none for an open port', async () => {
    const simple = await mountScanned()
    expect(simple.get('[data-test="fix"]').text()).toBe(`How to fix it: ${FIX}`)
    expect(simple.findAll('[data-test="fix"]')).toHaveLength(1)
    useNetauditViewMode().set('advanced')
    const advanced = await mountScanned()
    expect(advanced.get('article[data-check="smb_v1"] [data-test="fix"]').text()).toContain(FIX)
    expect(advanced.get('[data-key="p"]').find('[data-test="fix"]').exists()).toBe(false)
  })

  it('"Scan it again" scans the one device that ran out of time and says it started', async () => {
    const w = await mountScanned()
    await button(w, 'Scan it again').trigger('click')
    await flushPromises()
    expect(dispatcher.triggerAutomation).toHaveBeenCalledWith('ws', 'audit', undefined, '192.0.2.30')
    expect(w.get('[data-test="scan-notice"]').text()).toContain('192.0.2.30')
  })

  it('"Scan it again" runs the whole audit when several devices ran out of time', async () => {
    service.findings.mockResolvedValue({ ...scanned, unscanned: ['192.0.2.30', '192.0.2.31'] })
    const w = mount(FindingsReport, { props: { workspaceId: 'ws', automation: 'audit' } })
    await flushPromises()
    await button(w, 'Scan it again').trigger('click')
    expect(dispatcher.triggerAutomation).toHaveBeenCalledWith('ws', 'audit', undefined, undefined)
  })

  it('Advanced: "Rescan this host" scans just that device', async () => {
    useNetauditViewMode().set('advanced')
    const w = await mountScanned()
    await w.get('section[aria-label="192.0.2.10"] [data-test="rescan-host"]').trigger('click')
    await flushPromises()
    expect(dispatcher.triggerAutomation).toHaveBeenCalledWith('ws', 'audit', undefined, '192.0.2.10')
  })

  it('shows why a rescan could not start', async () => {
    dispatcher.triggerAutomation.mockRejectedValue(new Error('only a network audit can scan a device'))
    const w = await mountScanned()
    await button(w, 'Scan it again').trigger('click')
    await flushPromises()
    expect(w.get('[role="alert"]').text()).toBe('only a network audit can scan a device')
  })
})

describe('ScanSettingsFields target warning', () => {
  it('names the typed networks the audit would refuse, and why, once typing pauses', async () => {
    vi.useFakeTimers()
    try {
      service.checkTargets.mockResolvedValue([{ entry: '10.9.9.9', reason: 'It is outside the networks allowed for scanning.' }])
      useNetauditViewMode().set('advanced')
      const w = mount(ScanSettingsFields, { props: { targets: '', depth: 'thorough' as const, ports: '', checks: true, knownVulns: true, 'onUpdate:targets': (v: string) => w.setProps({ targets: v }) } })
      expect(w.find('[data-test="target-problems"]').exists()).toBe(false)
      await w.get('input[placeholder="every allowed network"]').setValue('10.9.9.9')
      await vi.advanceTimersByTimeAsync(500)
      expect(service.checkTargets).toHaveBeenCalledWith(['10.9.9.9'])
      const warning = w.get('[data-test="target-problems"]')
      expect(warning.text()).toContain('10.9.9.9: It is outside the networks allowed for scanning.')
    } finally {
      vi.useRealTimers()
    }
  })
})

describe('ScanSettingsFields known vulnerabilities', () => {
  it('offers "Look for known vulnerabilities" in Simple and passes the choice on', async () => {
    const w = mount(ScanSettingsFields, { props: { targets: '', depth: 'thorough' as const, ports: '', checks: true, knownVulns: true } })
    const toggle = w.findAll('label').find((l) => l.text() === 'Look for known vulnerabilities')!.get('input[role="switch"]')
    await toggle.setValue(false)
    expect(w.emitted('update:knownVulns')?.[0]).toEqual([false])
    expect(w.text()).toContain('may be affected')
  })
})

describe('VulnDataCard', () => {
  async function mountCard() {
    const w = mount(VulnDataCard, { attachTo: document.body })
    await flushPromises()
    return w
  }
  const button = (w: Awaited<ReturnType<typeof mountCard>>, name: string) => w.findAll('button').find((b) => b.text() === name)

  it('Simple: says nothing is downloaded and offers one button, which starts the download', async () => {
    service.startVulnDownload.mockResolvedValue(vulnStatus({ state: 'running' }))
    const w = await mountCard()
    expect(w.text()).toContain('Not downloaded')
    await button(w, 'Download')!.trigger('click')
    expect(service.startVulnDownload).toHaveBeenCalledTimes(1)
    expect(w.text()).toContain('Downloading file 1 of 6')
    w.unmount()
  })

  it('shows the attribution and says a match is not proof', async () => {
    const w = await mountCard()
    expect(w.text()).toContain('National Vulnerability Database')
    expect(w.text()).toContain("CISA's Known Exploited Vulnerabilities")
    expect(w.text()).toContain('may be affected')
    expect(w.text()).toContain('not proof')
    w.unmount()
  })

  it('shows a stopped download with the plain reason, and resumes it', async () => {
    service.vulnData.mockResolvedValue(vulnStatus({ state: 'stopped', files_done: 3, reason: 'The connection dropped.' }))
    service.startVulnDownload.mockResolvedValue(vulnStatus({ state: 'running', files_done: 3 }))
    const w = await mountCard()
    expect(w.text()).toContain('Stopped after 3 of 6 files')
    expect(w.text()).toContain('The connection dropped.')
    await button(w, 'Resume')!.trigger('click')
    expect(service.startVulnDownload).toHaveBeenCalled()
    w.unmount()
  })

  it('while it runs: file N of M, the megabytes, a bar and Cancel; polls once a second and stops on unmount', async () => {
    vi.useFakeTimers()
    try {
      service.vulnData.mockResolvedValue(vulnStatus({ state: 'running', files_done: 1, file: 'NVD 2023', bytes_done: 10 * 1024 * 1024, bytes_total: 40 * 1024 * 1024 }))
      service.cancelVulnDownload.mockResolvedValue(vulnStatus({ state: 'stopped', files_done: 1, reason: 'Cancelled.' }))
      const w = mount(VulnDataCard, { attachTo: document.body })
      await flushPromises()
      expect(w.text()).toContain('Downloading file 2 of 6')
      expect(w.text()).toContain('10 MB of 40 MB')
      expect(w.get('[role="meter"]').attributes('aria-valuenow')).toBe('25')
      const polled = service.vulnData.mock.calls.length
      await vi.advanceTimersByTimeAsync(2000)
      expect(service.vulnData.mock.calls.length).toBeGreaterThan(polled)
      await button(w, 'Cancel')!.trigger('click')
      await flushPromises()
      expect(service.cancelVulnDownload).toHaveBeenCalled()
      expect(w.text()).toContain('Stopped after 1 of 6 files')
      service.vulnData.mockResolvedValue(vulnStatus({ state: 'running', files_done: 1 }))
      service.cancelVulnDownload.mockResolvedValue(vulnStatus({ state: 'running', files_done: 1 }))
      w.unmount()
      service.vulnData.mockClear()
      await vi.advanceTimersByTimeAsync(5000)
      expect(service.vulnData).not.toHaveBeenCalled()
    } finally {
      vi.useRealTimers()
    }
  })

  it('does not poll when nothing is running', async () => {
    vi.useFakeTimers()
    try {
      const w = mount(VulnDataCard)
      await flushPromises()
      service.vulnData.mockClear()
      await vi.advanceTimersByTimeAsync(5000)
      expect(service.vulnData).not.toHaveBeenCalled()
      w.unmount()
    } finally {
      vi.useRealTimers()
    }
  })

  it('Simple: up to date with the date and nothing to click, so nothing can be downloaded again by accident', async () => {
    service.vulnData.mockResolvedValue(vulnHeld())
    const w = await mountCard()
    expect(w.text()).toContain('Up to date')
    expect(w.text()).not.toContain('CISA exploited list')
    expect(w.findAll('button').map((b) => b.text())).toEqual(['Delete data'])
    expect(service.startVulnDownload).not.toHaveBeenCalled()
    w.unmount()
  })

  it('Simple: old data offers Update, which fetches it again', async () => {
    const longAgo = new Date(Date.now() - 70 * 86_400_000).toISOString()
    const old = vulnHeld()
    service.vulnData.mockResolvedValue({ ...old, nvd: { ...old.nvd, imported_at: longAgo }, kev: { ...old.kev, imported_at: longAgo } })
    service.startVulnDownload.mockResolvedValue({ ...old, state: 'running', files_done: 0 })
    const w = await mountCard()
    expect(w.text()).toContain('Old, downloaded')
    await button(w, 'Update')!.trigger('click')
    expect(service.startVulnDownload).toHaveBeenCalledWith(false)
    expect(w.text()).toContain('Downloading file 1 of 6')
    w.unmount()
  })

  it('Advanced: a row per source with its date and counts', async () => {
    useNetauditViewMode().set('advanced')
    service.vulnData.mockResolvedValue(vulnHeld())
    const w = await mountCard()
    expect(w.text()).toContain('CISA exploited list')
    expect(w.text()).toContain('NVD 2022–2026')
    expect(w.text()).toContain('160,000 CVEs')
    w.unmount()
  })

  it('says why the status could not be read, with a retry', async () => {
    service.vulnData.mockRejectedValueOnce(new Error('connection refused'))
    const w = await mountCard()
    expect(w.text()).toContain('connection refused')
    service.vulnData.mockResolvedValue(vulnStatus())
    await button(w, 'Retry')!.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('Not downloaded')
    w.unmount()
  })

  it('shows why a download could not start', async () => {
    service.startVulnDownload.mockRejectedValue(new Error('A download is already running.'))
    const w = await mountCard()
    await button(w, 'Download')!.trigger('click')
    await flushPromises()
    expect(w.get('[role="alert"]').text()).toContain('already running')
    w.unmount()
  })

  it('Advanced: "Start again" fetches every file again, and only when something is held', async () => {
    useNetauditViewMode().set('advanced')
    service.vulnData.mockResolvedValue(vulnStatus({ state: 'stopped', files_done: 3, reason: 'The download did not finish.' }))
    service.startVulnDownload.mockResolvedValue(vulnStatus({ state: 'running', files_done: 3 }))
    const w = await mountCard()
    await button(w, 'Start again')!.trigger('click')
    expect(service.startVulnDownload).toHaveBeenCalledWith(true)
    w.unmount()
  })

  it('offers no "Start again" in Simple, before anything is held, or while it runs', async () => {
    service.vulnData.mockResolvedValue(vulnStatus({ state: 'stopped', files_done: 3 }))
    let w = await mountCard()
    expect(button(w, 'Start again')).toBeUndefined()
    w.unmount()
    useNetauditViewMode().set('advanced')
    service.vulnData.mockResolvedValue(vulnStatus())
    w = await mountCard()
    expect(button(w, 'Start again')).toBeUndefined()
    w.unmount()
    service.vulnData.mockResolvedValue(vulnStatus({ state: 'running', files_done: 1 }))
    w = await mountCard()
    expect(button(w, 'Start again')).toBeUndefined()
    w.unmount()
  })

  describe('Delete data', () => {
    it('asks first, says what it costs, and deletes only when confirmed', async () => {
      service.vulnData.mockResolvedValue(vulnHeld())
      service.clearVulnData.mockResolvedValue(vulnStatus())
      confirmMock.mockReset().mockResolvedValueOnce(false).mockResolvedValueOnce(true)
      const w = await mountCard()
      await button(w, 'Delete data')!.trigger('click')
      await flushPromises()
      expect(confirmMock.mock.calls[0]![0].message).toContain('known-vulnerability check')
      expect(service.clearVulnData).not.toHaveBeenCalled()
      await button(w, 'Delete data')!.trigger('click')
      await flushPromises()
      expect(service.clearVulnData).toHaveBeenCalledTimes(1)
      expect(w.text()).toContain('Not downloaded')
      w.unmount()
    })

    it('is offered only when something is held, in both views', async () => {
      service.vulnData.mockResolvedValue(vulnStatus())
      let w = await mountCard()
      expect(button(w, 'Delete data')).toBeUndefined()
      w.unmount()
      service.vulnData.mockResolvedValue(vulnHeld())
      w = await mountCard()
      expect(button(w, 'Delete data')).toBeDefined()
      w.unmount()
      useNetauditViewMode().set('advanced')
      w = await mountCard()
      expect(button(w, 'Delete data')).toBeDefined()
      w.unmount()
    })

    it('is not offered while a download runs: cancel it first', async () => {
      service.vulnData.mockResolvedValue(vulnStatus({ state: 'running', files_done: 2 }))
      const w = await mountCard()
      expect(button(w, 'Delete data')).toBeUndefined()
      w.unmount()
    })
  })

  it('keeps the 44px touch target on its button', async () => {
    const w = await mountCard()
    expect(button(w, 'Download')!.classes().join(' ')).toContain('max-sm:!h-11')
    w.unmount()
  })
})

describe('ReadinessBanner vulnerability notice', () => {
  const notice = { code: 'vuln_data_missing', message: 'Vulnerability data is not downloaded, so audits skip the check.', blocking: false, fix: { kind: 'open_settings', section: 'network-scan' } }

  it('shows it as a note beside the real requirements, never as a warning, with the link to the card', async () => {
    service.readiness.mockResolvedValue({ ready: true, requirements: [], notices: [notice] })
    const w = mount(ReadinessBanner)
    await flushPromises()
    expect(w.text()).toContain('Known-vulnerability data is not downloaded')
    expect(w.get('[data-tone]').attributes('data-tone')).toBe('info')
    await w.findAll('button').find((b) => b.text() === 'Open settings')!.trigger('click')
    expect(push).toHaveBeenCalledWith({ name: expect.any(String), params: { section: 'network-scan' } })
  })

  it('does not link to the page it is already on', async () => {
    service.readiness.mockResolvedValue({ ready: true, requirements: [], notices: [notice] })
    const w = mount(ReadinessBanner, { props: { here: 'network-scan' } })
    await flushPromises()
    expect(w.text()).not.toContain('Open settings')
  })

  it('shows a one-line progress while a download runs', async () => {
    service.readiness.mockResolvedValue({ ready: true, requirements: [], notices: [notice] })
    service.vulnData.mockResolvedValue(vulnStatus({ state: 'running', files_done: 1 }))
    const w = mount(ReadinessBanner)
    await flushPromises()
    expect(w.text()).toContain('Downloading vulnerability data, file 2 of 6')
    w.unmount()
  })

  describe('when the data is old', () => {
    const old = { code: 'vuln_data_old', message: 'The vulnerability data was downloaded 70 days ago.', blocking: false, fix: { kind: 'open_settings', section: 'network-scan' } }
    const mountOld = async (here?: 'network-scan') => {
      service.readiness.mockResolvedValue({ ready: true, requirements: [], notices: [old] })
      service.vulnData.mockResolvedValue(vulnHeld())
      const w = mount(ReadinessBanner, { props: { here } })
      await flushPromises()
      return w
    }
    const update = (w: Awaited<ReturnType<typeof mountOld>>) => w.findAll('button').find((b) => b.text() === 'Update now')

    it('says so and updates in place from "Update now", then shows the progress', async () => {
      service.startVulnDownload.mockResolvedValue({ ...vulnHeld(), state: 'running', files_done: 0 })
      const w = await mountOld()
      expect(w.text()).toContain('Known-vulnerability data is out of date')
      expect(w.get('[data-tone]').attributes('data-tone')).toBe('info')
      await update(w)!.trigger('click')
      expect(service.startVulnDownload).toHaveBeenCalledWith(false)
      await flushPromises()
      expect(w.text()).toContain('Downloading vulnerability data, file 1 of 6')
      expect(update(w)).toBeUndefined()
      w.unmount()
    })

    it('also offers it on the settings page itself, where the card is, without a link to itself', async () => {
      const w = await mountOld('network-scan')
      expect(update(w)).toBeDefined()
      expect(w.text()).not.toContain('Open settings')
      w.unmount()
    })

    it('says why the update could not start', async () => {
      service.startVulnDownload.mockRejectedValue(new Error('Internet access is off.'))
      const w = await mountOld()
      await update(w)!.trigger('click')
      await flushPromises()
      expect(w.get('[role="alert"]').text()).toContain('Internet access is off.')
      w.unmount()
    })

    it('has no "Update now" on the notices that are not about old data', async () => {
      service.readiness.mockResolvedValue({ ready: true, requirements: [], notices: [{ ...old, code: 'vuln_data_missing' }] })
      const w = mount(ReadinessBanner)
      await flushPromises()
      expect(update(w)).toBeUndefined()
      w.unmount()
    })
  })

  it('shows nothing, and asks nothing about the download, when there is no notice', async () => {
    useNetauditReadiness().readiness.value = null
    const w = mount(ReadinessBanner)
    await flushPromises()
    expect(w.find('[data-test="readiness"]').exists()).toBe(false)
    expect(service.vulnData).not.toHaveBeenCalled()
  })
})

describe('ReadinessBanner', () => {
  const missing = {
    ready: false,
    requirements: [
      { code: 'scope_empty', message: 'No network allowed.', blocking: true, fix: { kind: 'open_settings', section: 'network-scan' } },
      { code: 'nmap_missing', message: 'nmap is not available.', blocking: false, fix: { kind: 'copy', text: 'brew install nmap' } },
    ],
  }

  it('shows nothing when scans can run', async () => {
    const w = mount(ReadinessBanner)
    await flushPromises()
    expect(w.find('[data-test="readiness"]').exists()).toBe(false)
  })

  it('names each missing step in plain words, with its fix', async () => {
    service.readiness.mockResolvedValue(missing)
    const w = mount(ReadinessBanner)
    await flushPromises()
    expect(w.text()).toContain('Choose which network to look at')
    expect(w.text()).toContain('brew install nmap')
    await w.findAll('button').find((b) => b.text() === 'Open settings')!.trigger('click')
    expect(push).toHaveBeenCalledWith({ name: expect.any(String), params: { section: 'network-scan' } })
  })

  it('does not link to the page it is already on', async () => {
    service.readiness.mockResolvedValue(missing)
    const w = mount(ReadinessBanner, { props: { here: 'network-scan' } })
    await flushPromises()
    expect(w.text()).not.toContain('Open settings')
  })

  it('shows the exact message in Advanced', async () => {
    service.readiness.mockResolvedValue(missing)
    useNetauditViewMode().set('advanced')
    const w = mount(ReadinessBanner)
    await flushPromises()
    expect(w.text()).toContain('No network allowed.')
  })

  it('opens Help on the right part from "Learn more"', async () => {
    service.readiness.mockResolvedValue({ ready: false, requirements: [{ code: 'no_sandbox', message: 'x', blocking: true, fix: { kind: 'confirm', action: 'allow_unconfined' } }] })
    const w = mount(ReadinessBanner)
    await flushPromises()
    await w.findAll('button').find((b) => b.text() === 'Learn more')!.trigger('click')
    expect(useNetauditHelp().open.value).toBe(true)
    expect(useNetauditHelp().topic.value).toBe('safety')
  })
})

describe('NetworkScanSettings', () => {
  it('has the vulnerability data card under the allowed networks', async () => {
    service.scope.mockResolvedValue(emptyScope)
    const w = mount(NetworkScanSettings)
    await flushPromises()
    expect(w.text()).toContain('Vulnerability data')
    expect(w.text()).toContain('Not downloaded')
  })

  it('turns scanning on with one button in the Simple view', async () => {
    service.scope.mockResolvedValue(emptyScope)
    service.updateScope.mockResolvedValue({ ...emptyScope, ranges: [{ cidr: lan.cidr, devices: 254 }], suggestions: [lan] })
    const w = mount(NetworkScanSettings)
    await flushPromises()
    expect(w.text()).toContain('Scanning is off')
    expect(w.text()).not.toContain('Allow scans without file confinement')
    await w.findAll('button').find((b) => b.text() === 'Use my network')!.trigger('click')
    await flushPromises()
    expect(service.updateScope).toHaveBeenCalledWith({ entries: ['192.168.1.0/24'] })
    expect(w.text()).toContain('Your network')
  })

  it('adds a typed range in Advanced and shows the server\'s refusal', async () => {
    service.scope.mockResolvedValue(emptyScope)
    service.updateScope.mockRejectedValue(new Error('8.8.8.8 is a public address'))
    useNetauditViewMode().set('advanced')
    const w = mount(NetworkScanSettings)
    await flushPromises()
    expect(w.text()).toContain('Allow scans without file confinement')
    await w.get('input[type="text"]').setValue('8.8.8.8')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(service.updateScope).toHaveBeenCalledWith({ entries: ['8.8.8.8'] })
    expect(w.get('[role="alert"]').text()).toContain('public address')
  })

  it('opens the ordinary new-automation form from the nightly audit playbook, once a network is allowed', async () => {
    service.scope.mockResolvedValue({ ...emptyScope, ranges: [{ cidr: lan.cidr, devices: 254 }] })
    const w = mount(NetworkScanSettings)
    await flushPromises()
    await w.findAll('button').find((b) => b.text() === 'Set up a network audit')!.trigger('click')
    expect(push).toHaveBeenCalledWith({ name: expect.any(String), query: expect.objectContaining({ playbook: 'network-audit' }) })
  })

  const audit = (over: Record<string, unknown> = {}) => ({
    id: 'ws/audit', workspace: 'ws', name: 'audit', task_file: 'audit.md', strategy: 'persistent', trigger: 'cron', trigger_value: '0 2 * * *',
    profile: 'security-auditor', history: [{ id: 'r1', timestamp: '2026-10-25T02:00:00Z' }], ...over,
  })

  it('shows the audit that exists instead of offering to set one up, with a way to open it', async () => {
    service.scope.mockResolvedValue({ ...emptyScope, ranges: [{ cidr: lan.cidr, devices: 254 }] })
    dispatcher.listAutomations.mockResolvedValue([audit(), audit({ id: 'ws/other', name: 'other', profile: 'research' })])
    const w = mount(NetworkScanSettings)
    await flushPromises()
    const list = w.get('[data-test="audits"]')
    expect(list.findAll('li')).toHaveLength(1)
    expect(list.text()).toContain('audit')
    expect(list.text()).toContain('Every day at 02:00')
    expect(list.text()).toMatch(/Last run/)
    expect(w.findAll('button').some((b) => b.text() === 'Set up a network audit')).toBe(false)
    expect(w.findAll('button').some((b) => b.text() === 'Add another audit')).toBe(false)
    await list.get('button').trigger('click')
    expect(push).toHaveBeenCalledWith({ name: expect.any(String), params: { id: 'ws/audit' } })
  })

  it('offers another audit in Advanced only, through the same form', async () => {
    service.scope.mockResolvedValue({ ...emptyScope, ranges: [{ cidr: lan.cidr, devices: 254 }] })
    dispatcher.listAutomations.mockResolvedValue([audit()])
    useNetauditViewMode().set('advanced')
    const w = mount(NetworkScanSettings)
    await flushPromises()
    await w.findAll('button').find((b) => b.text() === 'Add another audit')!.trigger('click')
    expect(push).toHaveBeenCalledWith({ name: expect.any(String), query: expect.objectContaining({ playbook: 'network-audit' }) })
  })

  it('does not offer to set up an audit before it knows whether one exists', async () => {
    service.scope.mockResolvedValue({ ...emptyScope, ranges: [{ cidr: lan.cidr, devices: 254 }] })
    dispatcher.listAutomations.mockReturnValue(new Promise(() => {}))
    const w = mount(NetworkScanSettings)
    await flushPromises()
    expect(w.findAll('button').some((b) => b.text() === 'Set up a network audit')).toBe(false)
  })

  it('keeps the nightly audit button off until a network is allowed', async () => {
    service.scope.mockResolvedValue(emptyScope)
    const w = mount(NetworkScanSettings)
    await flushPromises()
    expect(w.findAll('button').find((b) => b.text() === 'Set up a network audit')!.attributes('disabled')).toBeDefined()
  })

  it('offers the time allowed per device in Advanced only, and saves the choice at once', async () => {
    service.scope.mockResolvedValue(emptyScope)
    service.updateScope.mockResolvedValue({ ...emptyScope, device_timeout_minutes: 5 })
    const simple = mount(NetworkScanSettings)
    await flushPromises()
    expect(simple.text()).not.toContain('Time allowed per device')
    useNetauditViewMode().set('advanced')
    const w = mount(NetworkScanSettings)
    await flushPromises()
    expect(w.text()).toContain('Time allowed per device')
    expect(w.text()).toContain('A whole scan may take up to 12 minutes')
    await w.get('select[aria-label="Time allowed per device"]').setValue('5')
    await flushPromises()
    expect(service.updateScope).toHaveBeenCalledWith({ device_timeout_minutes: 5 })
    expect((w.get('select[aria-label="Time allowed per device"]').element as HTMLSelectElement).value).toBe('5')
  })

  it('scrolls to the time setting when the page is opened on its anchor', async () => {
    service.scope.mockResolvedValue(emptyScope)
    useNetauditViewMode().set('advanced')
    routeState.hash = '#scan-time'
    const scrolled = vi.fn()
    const original = Element.prototype.scrollIntoView
    Element.prototype.scrollIntoView = scrolled
    const w = mount(NetworkScanSettings, { attachTo: document.body })
    await flushPromises()
    Element.prototype.scrollIntoView = original
    expect(scrolled).toHaveBeenCalledTimes(1)
    expect(w.find('#scan-time').exists()).toBe(true)
    w.unmount()
  })

  it('says why the networks could not be read', async () => {
    service.scope.mockRejectedValue(new Error('down'))
    const w = mount(NetworkScanSettings)
    await flushPromises()
    expect(w.text()).toContain('down')
  })
})

describe('NetauditHelp', () => {
  it('opens on the asked part and closes with the button', async () => {
    const help = useNetauditHelp()
    const w = mount(NetauditHelp, { attachTo: document.body })
    expect(w.find('[role="dialog"]').exists()).toBe(false)
    help.show('words')
    await flushPromises()
    expect(w.text()).toContain('Port: a numbered door')
    const expanded = w.findAll('button[aria-expanded]').map((b) => [b.text(), b.attributes('aria-expanded')])
    expect(expanded).toContainEqual(['Words explained', 'true'])
    expect(expanded).toContainEqual(['The basics', 'false'])
    await w.get('button[aria-label="Close help"]').trigger('click')
    expect(help.open.value).toBe(false)
    w.unmount()
  })
})

describe('NetauditHelp known vulnerabilities', () => {
  it('explains what matching can and cannot tell you', async () => {
    const help = useNetauditHelp()
    const w = mount(NetauditHelp, { attachTo: document.body })
    help.show('vulns')
    await flushPromises()
    expect(w.text()).toContain('may be affected')
    expect(w.text()).toContain('National Vulnerability Database')
    w.unmount()
  })
})

describe('NetauditHelp fixing a finding', () => {
  it('says the app changes nothing and that a mark never hides a finding', async () => {
    const help = useNetauditHelp()
    const w = mount(NetauditHelp, { attachTo: document.body })
    help.show('fixes')
    await flushPromises()
    expect(w.text()).toContain('never changes a device')
    expect(w.text()).toContain('never hides it')
    w.unmount()
  })
})

describe('AuditSetupCheck', () => {
  it('says nothing for an agent that is not the auditor', async () => {
    const w = mount(AuditSetupCheck, { props: { profile: 'research', networkGrant: 'none' } })
    await flushPromises()
    expect(w.find('[data-test="audit-setup"]').exists()).toBe(false)
    expect(service.readiness).not.toHaveBeenCalled()
  })

  it('warns when the automation\'s network access takes the scan tool away, and offers the fix in the form', async () => {
    for (const grant of ['none', 'internet_only']) {
      const w = mount(AuditSetupCheck, { props: { profile: 'security-auditor', networkGrant: grant, fixable: true } })
      await flushPromises()
      expect(w.text()).toContain("can't reach your network")
      await w.findAll('button').find((b) => b.text().includes('Local network only'))!.trigger('click')
      expect(w.emitted('use-local-network')).toHaveLength(1)
    }
  })

  it('stays quiet about network access when it is fine, and links to the editor outside the form', async () => {
    const fine = mount(AuditSetupCheck, { props: { profile: 'security-auditor', networkGrant: 'lan' } })
    await flushPromises()
    expect(fine.text()).not.toContain("can't reach your network")
    const bad = mount(AuditSetupCheck, { props: { profile: 'security-auditor', networkGrant: 'none', automationId: 'ws/audit' } })
    await flushPromises()
    await bad.findAll('button').find((b) => b.text() === 'Edit automation')!.trigger('click')
    expect(push).toHaveBeenCalledWith({ name: expect.any(String), params: { id: 'ws/audit' } })
  })

  it('shows what the computer is missing for an audit', async () => {
    service.readiness.mockResolvedValue({ ready: false, requirements: [{ code: 'scope_empty', message: 'x', blocking: true, fix: { kind: 'open_settings', section: 'network-scan' } }] })
    const w = mount(AuditSetupCheck, { props: { profile: 'security-auditor', networkGrant: 'lan' } })
    await flushPromises()
    expect(w.text()).toContain('Choose which network to look at')
  })
})

describe('FailureExplanation', () => {
  const OUT = 'agent execution failed: tool unavailable: operator action required: 192.168.1.0/24: target is outside the declared scan ranges'

  it('says what went wrong and what is allowed, with the original one tap away', async () => {
    service.scope.mockResolvedValue({ ...emptyScope, ranges: [{ cidr: '192.168.1.0/24', devices: 254 }] })
    const w = mount(FailureExplanation, { props: { error: OUT, automationId: 'ws/audit' } })
    await flushPromises()
    expect(w.text()).toContain("That network isn't allowed for scanning")
    expect(w.text()).toContain('192.168.1.0/24')
    expect(w.text()).not.toContain('operator action required')
    await w.findAll('button').find((b) => b.text() === 'Show details')!.trigger('click')
    expect(w.text()).toContain('operator action required')
    await w.findAll('button').find((b) => b.text() === 'Open network settings')!.trigger('click')
    expect(push).toHaveBeenCalledWith({ name: expect.any(String), params: { section: 'network-scan' } })
  })

  it('shows a failure it does not know exactly as it came', async () => {
    const w = mount(FailureExplanation, { props: { error: 'the model timed out' } })
    await flushPromises()
    expect(w.text()).toBe('the model timed out')
    expect(service.scope).not.toHaveBeenCalled()
  })
})

describe('ScanSummary', () => {
  const RouterLink = { props: ['to'], template: '<a :data-to="JSON.stringify(to)"><slot /></a>' }
  const render = (scan?: ScanSettings) =>
    mount(ScanSummary, { props: { scan }, global: { stubs: { RouterLink } } })

  it('shows the defaults as clear tags, with the allowed networks one click away', () => {
    const w = render(undefined)
    const tags = w.findAll('[data-test="scan-tag"]').map((t) => t.text())
    expect(tags).toEqual(['Thorough scan', 'Safe checks on'])
    const link = w.get('a')
    expect(link.text()).toBe('Every allowed network')
    expect(JSON.parse(link.attributes('data-to')!)).toMatchObject({ params: { section: 'network-scan' } })
  })

  it('shows what was chosen: quick, checks off, the ports and each network on its own', () => {
    const w = render({ depth: 'quick', checks: false, ports: '22,80', targets: ['192.168.1.0/24', 'nas.local'] })
    expect(w.findAll('[data-test="scan-tag"]').map((t) => t.text())).toEqual(['Quick scan', 'Checks off', 'ports 22,80', '192.168.1.0/24', 'nas.local'])
    expect(w.find('a').exists()).toBe(false)
  })
})
