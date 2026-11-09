import { beforeEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import FindingsSimple from '../../../components/netaudit/FindingsSimple.vue'
import { useProblemLayout } from '../../../composables/netaudit/useProblemLayout'
import { summarize } from '../../../utils/netaudit/report'
import type { FindingsReport, ScanFinding } from '../../../types/netaudit'

const problem = (key: string, severity: string, over: Partial<ScanFinding> = {}): ScanFinding => ({
  key, device: `192.0.2.${key.length}`, port: 0, check: `c-${key}`, check_title: `Title ${key}`, severity, evidence: `Evidence ${key}`, new: false, changed: false, accepted: false, ...over,
})
const report = (findings: ScanFinding[]): FindingsReport => ({ scanned: true, findings, resolved: [], unscanned: [], devices_up: 3 })
const mountWith = (findings: ScanFinding[], props: Record<string, unknown> = {}) => {
  const r = report(findings)
  return mount(FindingsSimple, { props: { report: r, summary: summarize(r), ...props } })
}

// The quiet findings stay out of the way (vertex, 2026-11-09: nine identical low findings buried the one high one).
describe('FindingsSimple sections', () => {
  beforeEach(() => {
    useProblemLayout().set('check')
  })

  it('puts a serious or new problem under "Needs you" and a quiet one under "Notes"', () => {
    const w = mountWith([problem('aa', 'high'), problem('b', 'low'), problem('cc', 'low', { new: true })])
    const needs = w.get('[data-test="needs-you"]')
    expect(needs.findAll('[data-test="problem-title"]').map((t) => t.text())).toEqual(['Title aa', 'Title cc'])
    const notes = w.get('[data-test="notes"]')
    expect(notes.findAll('[data-test="problem-title"]').map((t) => t.text())).toEqual(['Title b'])
    expect(notes.get('[data-test="notes-heading"]').text()).toContain('1')
  })

  it('leaves out a section with nothing in it', () => {
    const quiet = mountWith([problem('b', 'low')])
    expect(quiet.find('[data-test="needs-you"]').exists()).toBe(false)
    expect(quiet.find('[data-test="notes"]').exists()).toBe(true)
    const none = mountWith([])
    expect(none.find('[data-test="notes"]').exists()).toBe(false)
    expect(none.find('[data-test="layout-switch"]').exists()).toBe(false)
  })

  it('switches to one card per device and remembers it', async () => {
    const w = mountWith([problem('aa', 'high'), problem('b', 'low')])
    expect(w.find('[data-test="problems-by-device"]').exists()).toBe(false)
    const device = w.findAll('[data-test="layout-switch"] [role="radio"]').find((b) => b.text() === 'By device')!
    await device.trigger('click')
    expect(w.find('[data-test="problems-by-device"]').exists()).toBe(true)
    expect(w.find('[data-test="needs-you"]').exists()).toBe(false)
    expect(useProblemLayout().layout.value).toBe('device')
  })

  it('passes a request to accept several problems up, from either layout', async () => {
    const many = [problem('a', 'low', { check: 'dns', device: '192.0.2.1' }), problem('b', 'low', { check: 'dns', device: '192.0.2.2' })]
    const w = mountWith(many)
    await w.get('[data-test="notes"] [data-test="card-toggle"]').trigger('click')
    await w.get('[data-test="accept-all"]').trigger('click')
    expect((w.emitted('accept-many')![0]![0] as ScanFinding[]).map((f) => f.key)).toEqual(['a', 'b'])
  })
})
