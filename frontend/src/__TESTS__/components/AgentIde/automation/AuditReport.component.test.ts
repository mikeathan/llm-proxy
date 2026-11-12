import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import AuditReport from '../../../../components/AgentIde/automation/AuditReport.vue'

const STUBS = { Icon: true, MarkdownViewer: { props: ['content'], template: '<div class="md">{{ content }}</div>' } }
const REPORT = [
  '## Observed (generated from the scan data, not written by the model)',
  '',
  'Scanned 254 address(es): 15 up.',
  '',
  '### Still open',
  '',
  '| Severity | Finding | Where |',
  '|---|---|---|',
  '| high | A program file no longer matches its package | this-computer |',
  '',
  'Suggested fixes:',
  '',
  '- **Program file**: Reinstall the package.',
  '',
  '### Resolved',
  '',
  '| Device | Port | Service | Version |',
  '|---|---|---|---|',
  '| 192.0.2.64 | 49152/tcp |  |  |',
  '',
  '> Note: 14 watched locations could not be read.',
  '',
  '---',
  '',
  '## Assessment (written by the model)',
  '',
  '1. The high finding first.',
].join('\n')

const mountReport = (content: string) => mount(AuditReport, { props: { content }, global: { stubs: STUBS } })

describe('AuditReport', () => {
  it('shows the assessment first, keeps what is still open in view and folds the rest with a summary', () => {
    const w = mountReport(REPORT)
    const text = w.text()
    expect(text.indexOf('Assessment')).toBeLessThan(text.indexOf('Still open'))
    expect(w.find('[data-test="audit-open-Still open"]').exists()).toBe(true)
    const resolved = w.get('[data-test="audit-folded-Resolved"]')
    expect(resolved.text()).toContain('1 port')
    expect(resolved.get('button[aria-expanded]').attributes('aria-expanded')).toBe('false')
    expect(w.get('[data-test="audit-notes"] button[aria-expanded]').attributes('aria-expanded')).toBe('false')
    expect(w.get('[data-test="audit-notes"]').text()).toContain('1 note')
  })

  it('folds the suggested fixes of an open section, one line saying how many', () => {
    const w = mountReport(REPORT)
    const fixes = w.get('[data-test="audit-open-Still open"] section')
    expect(fixes.text()).toContain('Suggested fixes')
    expect(fixes.text()).toContain('1 fix')
    expect(fixes.get('button[aria-expanded]').attributes('aria-expanded')).toBe('false')
  })

  it('opens a folded part when asked', async () => {
    const w = mountReport(REPORT)
    await w.get('[data-test="audit-folded-Resolved"] button[aria-expanded]').trigger('click')
    expect(w.get('[data-test="audit-folded-Resolved"] button[aria-expanded]').attributes('aria-expanded')).toBe('true')
  })

  it('shows a text that is not an audit report as plain markdown', () => {
    const w = mountReport('# Some other report\n\nBody.')
    expect(w.find('[data-test="audit-report"]').exists()).toBe(false)
    expect(w.get('.md').text()).toContain('Some other report')
  })

  it('starts the folds again when another report is shown', async () => {
    const w = mountReport(REPORT)
    await w.get('[data-test="audit-folded-Resolved"] button[aria-expanded]').trigger('click')
    await w.setProps({ content: REPORT.replace('Scanned 254', 'Scanned 100') })
    expect(w.get('[data-test="audit-folded-Resolved"] button[aria-expanded]').attributes('aria-expanded')).toBe('false')
  })
})
