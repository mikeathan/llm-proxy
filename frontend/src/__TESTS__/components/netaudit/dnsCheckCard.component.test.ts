import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { DnsCheckState } from '../../../types/netaudit'

const service = vi.hoisted(() => ({ dnsCheck: vi.fn(), setDnsCheck: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))
const clipboard = vi.hoisted(() => ({ copyText: vi.fn() }))
vi.mock('../../../utils/clipboard', () => clipboard)

import DnsCheckCard from '../../../components/netaudit/DnsCheckCard.vue'
import { useNetauditViewMode } from '../../../composables/netaudit/useNetauditViewMode'

const NEVER = '0001-01-01T00:00:00Z'
const state = (over: Partial<DnsCheckState> = {}, summary: Partial<DnsCheckState['summary']> = {}): DnsCheckState => ({
  enabled: false,
  interval_minutes: 60,
  allowed_intervals: [15, 30, 60, 120, 360],
  names: ['dns.google', 'one.one.one.one'],
  default_names: ['dns.google', 'one.one.one.one'],
  server: '',
  resolver_url: 'https://cloudflare-dns.com/dns-query',
  default_resolver_url: 'https://cloudflare-dns.com/dns-query',
  status: { last_run: NEVER },
  summary: { comparisons: 0, matches: 0, differs: 0, cannot_compare: 0, overlapping: 0, first: NEVER, last: NEVER, differences: [], ...summary },
  ...over,
})

async function mountCard() {
  const w = mount(DnsCheckCard, {
    attachTo: document.body,
    global: { stubs: { RouterLink: { props: ['to'], template: '<a :href="JSON.stringify(to)"><slot /></a>' } } },
  })
  await flushPromises()
  return w
}

describe('DnsCheckCard', () => {
  beforeEach(() => {
    service.dnsCheck.mockReset().mockResolvedValue(state())
    service.setDnsCheck.mockReset()
    clipboard.copyText.mockReset().mockResolvedValue(true)
    useNetauditViewMode().set('simple')
  })

  it('is off until the person turns it on, and says nothing about comparisons that never happened', async () => {
    const w = await mountCard()
    expect((w.get('[role="switch"]').element as HTMLInputElement).checked).toBe(false)
    expect(w.find('[data-test="dns-check-status"]').exists()).toBe(false)
    w.unmount()
  })

  it('tells the person, before they switch it on, that the outside resolver learns the names and the address', async () => {
    const w = await mountCard()
    await w.get('[data-test="toggle-details"]').trigger('click')
    const text = w.get('[data-test="toggle-details-text"]').text()
    expect(text).toContain('learns which names are asked and your public internet address')
    expect(text).toContain('not one that lies about a single site')
    w.unmount()
  })

  it('turns it on through the service and shows the state the server confirmed', async () => {
    service.setDnsCheck.mockResolvedValue(state({ enabled: true }))
    const w = await mountCard()
    await w.get('[role="switch"]').trigger('click')
    await flushPromises()
    expect(service.setDnsCheck).toHaveBeenCalledWith({ enabled: true })
    expect(w.get('[data-test="dns-check-status"]').text()).toContain('Nothing compared yet')
    w.unmount()
  })

  it('shows a failed change as an error and keeps the old state', async () => {
    service.setDnsCheck.mockRejectedValue(new Error('A passphrase is needed'))
    const w = await mountCard()
    await w.get('[role="switch"]').trigger('click')
    await flushPromises()
    expect(w.get('[role="alert"]').text()).toContain('passphrase')
    expect((w.get('[role="switch"]').element as HTMLInputElement).checked).toBe(false)
    w.unmount()
  })

  it('links to the guardrail settings when internet access is off, and says nothing was sent', async () => {
    service.dnsCheck.mockResolvedValue(state({ enabled: true, status: { last_run: NEVER, problem: 'internet_off' } }))
    const w = await mountCard()
    expect(w.get('[data-test="dns-check-status"]').text()).toContain('nothing was sent')
    expect(w.find('[data-test="dns-check-settings"]').exists()).toBe(true)
    w.unmount()
  })

  it('links to the guardrail settings when local network access is off, and says the router was not asked', async () => {
    service.dnsCheck.mockResolvedValue(state({ enabled: true, status: { last_run: NEVER, problem: 'lan_off' } }))
    const w = await mountCard()
    expect(w.get('[data-test="dns-check-status"]').text()).toContain('Local network access is off')
    expect(w.find('[data-test="dns-check-settings"]').exists()).toBe(true)
    w.unmount()
  })

  it('offers the names, interval and resolver only in Advanced, and sends them as one change', async () => {
    const w = await mountCard()
    expect(w.find('[data-test="dns-check-names"]').exists()).toBe(false)
    w.unmount()

    useNetauditViewMode().set('advanced')
    service.setDnsCheck.mockResolvedValue(state())
    const adv = await mountCard()
    await adv.get('[data-test="dns-check-names"]').setValue(' a.example \n\n b.example ')
    await adv.get('[data-test="dns-check-interval"]').setValue('30')
    await adv.get('[data-test="dns-check-url"]').setValue('https://dns.example.org/q')
    await adv.get('[data-test="dns-check-server"]').setValue('192.168.1.53')
    await adv.get('form').trigger('submit')
    await flushPromises()
    expect(service.setDnsCheck).toHaveBeenCalledWith({
      names: ['a.example', 'b.example'], server: '192.168.1.53', resolver_url: 'https://dns.example.org/q', interval_minutes: 30,
    })
    adv.unmount()
  })

  it('sends an empty list when the names are cleared, so the server goes back to the defaults', async () => {
    useNetauditViewMode().set('advanced')
    service.setDnsCheck.mockResolvedValue(state())
    const w = await mountCard()
    await w.get('[data-test="dns-check-names"]').setValue('  \n')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(service.setDnsCheck.mock.calls[0]![0].names).toEqual([])
    w.unmount()
  })

  it('copies the summary to bring back, and says whether it worked', async () => {
    useNetauditViewMode().set('advanced')
    service.dnsCheck.mockResolvedValue(state({ enabled: true }, { comparisons: 3, matches: 3 }))
    const w = await mountCard()
    await w.get('[data-test="dns-check-copy"]').trigger('click')
    await flushPromises()
    const copiedReport = JSON.parse(clipboard.copyText.mock.calls[0]![0] as string)
    expect(copiedReport.summary.comparisons).toBe(3)
    expect(copiedReport).toMatchObject({ names: ['dns.google', 'one.one.one.one'], server: 'default gateway', interval_minutes: 60 })
    expect(w.get('[data-test="dns-check-copy-note"]').text()).toBe('Copied')
    clipboard.copyText.mockResolvedValue(false)
    await w.get('[data-test="dns-check-copy"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test="dns-check-copy-note"]').text()).toBe('Copy failed')
    w.unmount()
  })

  describe('showing that it is alive and what differed', () => {
    const ago = (minutes: number) => new Date(Date.now() - minutes * 60_000).toISOString()

    it('says when it last checked, so a person can see it is running without copying anything', async () => {
      service.dnsCheck.mockResolvedValue(state({ enabled: true, status: { last_run: ago(12) } }, { comparisons: 6, matches: 6, last: ago(12), first: ago(60) }))
      const w = await mountCard()
      const line = w.get('[data-test="dns-check-fresh"]')
      expect(line.text()).toContain('Last checked')
      expect(line.text()).toContain('12 minutes ago')
      expect(line.attributes('role')).toBeUndefined()
      w.unmount()
    })

    it('warns as an alert when no check was made for more than twice the interval', async () => {
      service.dnsCheck.mockResolvedValue(state({ enabled: true, status: { last_run: ago(300) } }, { comparisons: 6, matches: 6, last: ago(300), first: ago(400) }))
      const w = await mountCard()
      const line = w.get('[data-test="dns-check-fresh"]')
      expect(line.text()).toContain('may have stopped')
      expect(line.attributes('role')).toBe('alert')
      w.unmount()
    })

    it('shows nothing about the last check while it is switched off', async () => {
      service.dnsCheck.mockResolvedValue(state({ enabled: false, status: { last_run: ago(5) } }))
      const w = await mountCard()
      expect(w.find('[data-test="dns-check-fresh"]').exists()).toBe(false)
      w.unmount()
    })

    it('lists the newest differences with both address lists, in the simple view, when there are any', async () => {
      const differences = [{ at: ago(3), name: 'dns.google', outcome: 'differ' as const, overlap: true, router: ['10.0.0.1', '8.8.8.8'], outside: ['8.8.8.8', '8.8.4.4'] }]
      service.dnsCheck.mockResolvedValue(state({ enabled: true, status: { last_run: ago(3) } }, { comparisons: 6, matches: 5, differs: 1, last: ago(3), first: ago(60), differences }))
      const w = await mountCard()
      const rows = w.findAll('[data-test="dns-check-difference"]')
      expect(rows).toHaveLength(1)
      expect(rows[0]!.text()).toContain('dns.google')
      expect(rows[0]!.text()).toContain('10.0.0.1, 8.8.8.8')
      expect(rows[0]!.text()).toContain('8.8.8.8, 8.8.4.4')
      expect(rows[0]!.text()).toContain('partly differed')
      expect(w.get('[data-test="dns-check-differences"]').text()).toContain('not proof of a problem')
      w.unmount()
    })

    it('shows no differences section when nothing differed', async () => {
      service.dnsCheck.mockResolvedValue(state({ enabled: true, status: { last_run: ago(3) } }, { comparisons: 6, matches: 6 }))
      const w = await mountCard()
      expect(w.find('[data-test="dns-check-differences"]').exists()).toBe(false)
      w.unmount()
    })
  })
})
