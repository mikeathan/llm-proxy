import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { HttpError } from '../../../services/httpClient'
import type { DeepCheck, FindingsReport as Report, ScanFinding } from '../../../types/netaudit'

const service = vi.hoisted(() => ({ findings: vi.fn(), expectations: vi.fn(), deepChecks: vi.fn(), runDeepCheck: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))
const dispatcher = vi.hoisted(() => ({ triggerAutomation: vi.fn() }))
vi.mock('../../../services/automation/dispatcherService', () => ({ DispatcherService: dispatcher }))

import FindingsReport from '../../../components/netaudit/FindingsReport.vue'
import NetauditHelp from '../../../components/netaudit/NetauditHelp.vue'
import { useNetauditViewMode } from '../../../composables/netaudit/useNetauditViewMode'
import { useNetauditHelp } from '../../../composables/netaudit/useNetauditHelp'

const finding = (over: Partial<ScanFinding>): ScanFinding => ({
  key: 'k', device: '192.0.2.10', port: 22, proto: 'tcp', service: 'ssh', check: 'open_port', severity: 'info', new: false, changed: false, accepted: false, ...over,
})
const report: Report = {
  scanned: true, scanned_at: '2026-10-30T02:00:00Z', engine: 'nmap', devices_up: 2, resolved: [], unscanned: [],
  findings: [finding({ key: 'a', port: 22 }), finding({ key: 'b', port: 443, service: 'https' }), finding({ key: 'c', device: '192.0.2.11', port: 3389, service: 'ms-wbt-server' })],
}
const GUIDE = { steps: ['Put your public key on the account.'], risk: 'You can lock yourself out.', undo: 'Put the saved sshd_config back.' }
const found: DeepCheck = {
  device: '192.0.2.10', at: '2026-10-30T12:00:00Z',
  ports: [{ port: 22, tls_answered: false, ssh_answered: true }, { port: 443, tls_answered: false, ssh_answered: false }],
  findings: [{ device: '192.0.2.10', port: 22, proto: 'tcp', service: 'ssh', check: 'ssh_password_login', check_title: 'SSH accepts password login', severity: 'medium', evidence: 'The SSH server accepts password login', fix: 'Turn password login off.', guide: GUIDE }],
}

async function mountReport() {
  const w = mount(FindingsReport, { props: { workspaceId: 'ws', automation: 'audit' }, attachTo: document.body })
  await flushPromises()
  return w
}
const deepButton = (w: Awaited<ReturnType<typeof mountReport>>, device: string) => w.get(`section[aria-label="${device}"] [data-test="deep-check"]`)

describe('the deep check on a device', () => {
  beforeEach(() => {
    for (const fn of Object.values(service)) fn.mockReset()
    service.findings.mockResolvedValue(report)
    service.expectations.mockResolvedValue([])
    service.deepChecks.mockResolvedValue([])
    useNetauditViewMode().set('advanced')
  })
  afterEach(() => { document.body.innerHTML = '' })

  it('is offered only in the Advanced view, one button per device', async () => {
    const advanced = await mountReport()
    expect(advanced.findAll('[data-test="deep-check"]')).toHaveLength(2)
    useNetauditViewMode().set('simple')
    const simple = await mountReport()
    expect(simple.find('[data-test="deep-check"]').exists()).toBe(false)
  })

  it('asks first, says exactly what it will send, and starts nothing until the button in the dialog is pressed', async () => {
    const w = await mountReport()
    await deepButton(w, '192.0.2.10').trigger('click')
    const dialog = w.get('[data-test="deep-dialog"]')
    expect(dialog.text()).toContain('Deep check of 192.0.2.10?')
    expect(dialog.text()).toContain('2 open ports')
    expect(dialog.text()).toContain('22, 443')
    expect(dialog.get('[data-test="deep-sends"]').text()).toContain('about 60 connections')
    expect(dialog.get('[data-test="deep-sends"]').text()).toContain('made-up user name')
    expect(dialog.get('[data-test="deep-sends"]').text()).toContain('Guess no passwords')
    expect(dialog.text()).toContain('asked for your passphrase')
    expect(service.runDeepCheck).not.toHaveBeenCalled()
    await w.findAll('button').find((b) => b.text() === 'Cancel')!.trigger('click')
    expect(w.find('[data-test="deep-dialog"]').exists()).toBe(false)
    expect(service.runDeepCheck).not.toHaveBeenCalled()
  })

  it('runs one device, shows it as checking, then shows the result with its fix and steps', async () => {
    let finish: (c: DeepCheck) => void = () => undefined
    service.runDeepCheck.mockReturnValue(new Promise<DeepCheck>((resolve) => { finish = resolve }))
    const w = await mountReport()
    await deepButton(w, '192.0.2.10').trigger('click')
    await w.get('[data-test="deep-run"]').trigger('click')
    expect(service.runDeepCheck).toHaveBeenCalledWith('ws', 'audit', '192.0.2.10')
    expect(w.get('[data-test="deep-running"]').text()).toContain('192.0.2.10')
    expect(deepButton(w, '192.0.2.11').attributes('disabled')).toBeDefined() // one at a time
    finish(found)
    await flushPromises()
    expect(w.find('[data-test="deep-dialog"]').exists()).toBe(false)
    const result = w.get('[data-test="deep-result"]')
    expect(result.text()).toContain('1 problem found.')
    expect(result.get('[data-test="deep-finding"]').text()).toContain('SSH accepts password login')
    expect(result.text()).toContain('Turn password login off.')
    expect(result.text()).toContain('Put your public key on the account.')
  })

  it('says a device with no TLS or SSH service gave it nothing to check, and not that it is fine', async () => {
    service.deepChecks.mockResolvedValue([{ device: '192.0.2.11', at: '2026-10-30T12:00:00Z', ports: [{ port: 3389, tls_answered: false, ssh_answered: false }], findings: [] }])
    const w = await mountReport()
    const result = w.get('section[aria-label="192.0.2.11"] [data-test="deep-result"]')
    expect(result.text()).toContain('Nothing to check')
    expect(result.text()).toContain('not the same as nothing wrong')
    expect(result.text()).not.toContain('Nothing wrong was found')
  })

  it('holds a device back in its cool-down, and says why', async () => {
    service.deepChecks.mockResolvedValue([{ ...found, at: new Date(Date.now() - 2 * 60_000).toISOString() }])
    const w = await mountReport()
    const button = deepButton(w, '192.0.2.10')
    expect(button.attributes('disabled')).toBeDefined()
    expect(button.attributes('title')).toContain('Try again in')
    expect(deepButton(w, '192.0.2.11').attributes('disabled')).toBeUndefined()
  })

  it('keeps the earlier result and explains a refusal in the dialog', async () => {
    service.deepChecks.mockResolvedValue([{ ...found, at: '2026-10-01T12:00:00Z' }])
    service.runDeepCheck.mockRejectedValue(new HttpError('another scan is already running', '', 409))
    const w = await mountReport()
    await deepButton(w, '192.0.2.10').trigger('click')
    await w.get('[data-test="deep-run"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test="deep-error"]').text()).toContain('Another scan is running')
    expect(w.find('[data-test="deep-dialog"]').exists()).toBe(true)
    expect(w.find('[data-test="deep-result"]').exists()).toBe(true)
  })
})

describe('the Help for the deep check', () => {
  it('says what is sent, that it is the one exception and that nothing found is not nothing wrong', async () => {
    useNetauditHelp().show('deep')
    const w = mount(NetauditHelp, { attachTo: document.body })
    await flushPromises()
    const text = w.text()
    expect(text).toContain('The deep check')
    expect(text).toContain('about 60 connections')
    expect(text).toContain('made-up user name')
    expect(text).toContain('not the same as nothing being wrong')
    expect(text).toContain('the one exception')
    useNetauditHelp().hide()
  })
})
