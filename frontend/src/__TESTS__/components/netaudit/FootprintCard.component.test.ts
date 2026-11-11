import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const service = vi.hoisted(() => ({ footprint: vi.fn(), changeFootprint: vi.fn(), setFootprintKey: vi.fn(), clearFootprintKey: vi.fn(), checkFootprint: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

import FootprintCard from '../../../components/netaudit/FootprintCard.vue'

const state = (over = {}) => ({
  watches: [
    { label: 'domain:example.com', name: 'example.com', kind: 'domain', frequency: 'daily', results: [{ check: 'mail_records', status: 'looked', at: '2026-11-10T11:00:00Z', looked_at: '2026-11-10T11:00:00Z', spf: 'weak', dmarc: 'ok' }] },
    { label: 'email:a1', name: 'j***@example.com', kind: 'email', frequency: 'manual', results: [] },
  ],
  services: [
    { id: 'certificates', on: false, learns: 'The public certificate log (crt.sh) learns each domain you watch.' },
    { id: 'breaches', on: true, learns: 'The breach service learns only the first six characters of a hash.', needs_key: true },
  ],
  key_saved: false, max_domains: 10, max_emails: 10, ...over,
})

describe('FootprintCard', () => {
  beforeEach(() => { Object.values(service).forEach((f) => f.mockReset()); service.footprint.mockResolvedValue(state()) })

  it('shows what each service learns, the masked address and the results in plain words, and warns that breaches need a key', async () => {
    const w = mount(FootprintCard)
    await flushPromises()
    expect(w.get('[data-test="footprint-services"]').text()).toContain('crt.sh')
    expect(w.text()).toContain('j***@example.com')
    expect(w.text()).not.toContain('jo@example.com')
    expect(w.get('[data-test="result-mail_records"]').text()).toContain('SPF: weak. DMARC: ok.')
    expect(w.find('[data-test="needs-key"]').exists()).toBe(true)
  })

  it('asks for a look at one watch, and a switch change asks the server', async () => {
    service.checkFootprint.mockResolvedValue(state())
    service.changeFootprint.mockResolvedValue(state())
    const w = mount(FootprintCard)
    await flushPromises()
    await w.findAll('[data-test="watch-check"]')[0]!.trigger('click')
    expect(service.checkFootprint).toHaveBeenCalledWith('domain:example.com')
    await w.get('[data-test="service-certificates"]').setValue(true)
    expect(service.changeFootprint).toHaveBeenCalledWith({ action: 'service', service: 'certificates', on: true })
  })

  it('adds a domain and clears the box, and shows a refusal as it is', async () => {
    service.changeFootprint.mockResolvedValueOnce(state()).mockRejectedValueOnce(new Error('that domain is already watched'))
    const w = mount(FootprintCard)
    await flushPromises()
    await w.get('[data-test="footprint-domain"]').setValue('new.org')
    await w.get('[data-test="footprint-domain-add"]').trigger('submit')
    await flushPromises()
    expect(service.changeFootprint).toHaveBeenCalledWith({ action: 'add_domain', name: 'new.org' })
    expect((w.get('[data-test="footprint-domain"]').element as HTMLInputElement).value).toBe('')
    await w.get('[data-test="footprint-domain"]').setValue('dup.org')
    await w.get('[data-test="footprint-domain-add"]').trigger('submit')
    await flushPromises()
    expect(w.get('[data-test="footprint-error"]').text()).toContain('already watched')
  })
})
