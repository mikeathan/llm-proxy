import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { FindingsReport as Report, ScanFinding, ScanScope } from '../../../types/netaudit'

const service = vi.hoisted(() => ({ readiness: vi.fn(), scope: vi.fn(), updateScope: vi.fn(), findings: vi.fn(), accept: vi.fn(), revoke: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))
const dispatcher = vi.hoisted(() => ({ listAutomations: vi.fn(), listWorkspaces: vi.fn() }))
vi.mock('../../../services/automation/dispatcherService', () => ({ DispatcherService: dispatcher }))
const push = vi.hoisted(() => vi.fn())
const routeState = vi.hoisted(() => ({ hash: '' }))
vi.mock('vue-router', async (importOriginal) => ({ ...(await importOriginal<typeof import('vue-router')>()), useRouter: () => ({ push }), useRoute: () => routeState }))

import FindingsReport from '../../../components/netaudit/FindingsReport.vue'
import ReadinessBanner from '../../../components/netaudit/ReadinessBanner.vue'
import NetworkScanSettings from '../../../components/settings/NetworkScanSettings.vue'
import AuditSetupCheck from '../../../components/netaudit/AuditSetupCheck.vue'
import FailureExplanation from '../../../components/netaudit/FailureExplanation.vue'
import NetauditHelp from '../../../components/netaudit/NetauditHelp.vue'
import { useNetauditViewMode } from '../../../composables/netaudit/useNetauditViewMode'
import { useNetauditHelp } from '../../../composables/netaudit/useNetauditHelp'

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
const lan = { interface: 'en0', cidr: '192.168.1.0/24', devices: 254 }
const emptyScope: ScanScope = { ranges: [], allow_unconfined: false, device_timeout_minutes: 0, suggestions: [lan] }

beforeEach(() => {
  for (const fn of Object.values(service)) fn.mockReset()
  push.mockReset()
  dispatcher.listAutomations.mockReset().mockResolvedValue([])
  dispatcher.listWorkspaces.mockReset().mockResolvedValue([{ id: 'ws' }])
  routeState.hash = ''
  service.readiness.mockResolvedValue({ ready: true, requirements: [] })
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
    expect(tiles).toEqual(['2new since the last scan', '1changed', '1resolved', '1accepted (hidden from the assistant)'])
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
    expect(w.text()).toContain('Audits stop after 10 minutes in total')
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
    service.scope.mockResolvedValue({ ...emptyScope, ranges: [{ cidr: '192.168.50.0/24', devices: 254 }] })
    const w = mount(FailureExplanation, { props: { error: OUT, automationId: 'ws/audit' } })
    await flushPromises()
    expect(w.text()).toContain("That network isn't allowed for scanning")
    expect(w.text()).toContain('192.168.50.0/24')
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
