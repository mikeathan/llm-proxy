import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import GlobalSettings from '../../../components/settings/GlobalSettings.vue'
import { DEFAULT_CONFIG } from '../../../composables/models/useConfig'
import { fieldByLabel } from '../../helpers/fieldByLabel'
import type { GlobalConfig, SettingsTab } from '../../../types/admin'

vi.mock('../../../services/admin/adminService', () => ({ AdminApiService: { storage: vi.fn(() => new Promise(() => {})) } }))

// VTU's emitted() does not record script-setup emits in this harness, so the
// listener callbacks are the component's observable contract.
function mountGlobal(overrides: Partial<GlobalConfig> = {}, section: SettingsTab = 'memory') {
  const onUpdate = vi.fn()
  const onSave = vi.fn()
  const onLogLevel = vi.fn()
  const w = mount(GlobalSettings, {
    props: { editConfig: { ...structuredClone(DEFAULT_CONFIG), ...overrides }, logLevel: 'info', section },
    attrs: { 'onUpdate:editConfig': onUpdate, onUpdateConfig: onSave, onUpdateLogLevel: onLogLevel },
    global: { stubs: { Icon: true } },
  })
  const last = (): GlobalConfig => onUpdate.mock.lastCall![0]
  return { w, onUpdate, onSave, onLogLevel, last }
}

// General's sections each show only their own panels, all stay mounted. The model panels are on the Models page.
describe('GlobalSettings sections', () => {
  const visiblePanels = (w: ReturnType<typeof mountGlobal>['w']) =>
    w.findAll('section').filter((p) => !p.attributes('style')?.includes('display: none')).map((p) => p.get('h2').text().replace(/^\d+\s*—?\s*/, '').toLowerCase())

  it.each([
    ['memory', ['memory', 'reminders']],
    ['storage', ['workspace storage', 'what is stored', 'logging']],
  ] as const)('%s shows only its panels', (section, titles) => {
    expect(visiblePanels(mountGlobal({}, section).w)).toEqual(titles)
  })

  it('keeps every panel in the page, so an edit survives moving between sections', () => {
    const { w } = mountGlobal({}, 'memory')
    expect(w.findAll('section')).toHaveLength(4)
  })
})

// The toggle was labelled as if it kept only logs, so an operator looking for the exact request a model received did not
// know it was already there (recording.jsonl in each run's folder). It also says what is masked and how long it is kept.
describe('GlobalSettings run logging', () => {
  it('says it records the requests sent to the model, that secrets are masked and how long it is kept', () => {
    const { w } = mountGlobal({}, 'storage')
    const text = w.text()
    expect(text).toContain('Record run logs and model requests')
    expect(text).toContain('every request sent to the model')
    expect(text).toContain('recording.jsonl')
    expect(text).toMatch(/masked/i)
    expect(text).toContain('30 days')
    expect(text).toContain('/admin/api/recordings')
  })
})

describe('GlobalSettings', () => {
  it('writes the memory defaults as a pair, starting from assistant on and automations off', async () => {
    // The parent owns the config, so each interaction starts from the props as mounted.
    const { w, last } = mountGlobal()
    expect(fieldByLabel(w, /assistant remembers/i).element).toHaveProperty('checked', true)
    expect(fieldByLabel(w, /automations remember/i).element).toHaveProperty('checked', false)
    await fieldByLabel(w, /assistant remembers/i).setValue(false)
    expect(last().memory).toEqual({ assistant_hot: false, automation_hot: false })
    await fieldByLabel(w, /automations remember/i).setValue(true)
    expect(last().memory).toEqual({ assistant_hot: true, automation_hot: true })
  })

  it('turns run logging on', async () => {
    const { w, last } = mountGlobal({ run_logging: { enabled: false } }, 'storage')
    await fieldByLabel(w, /run logging/i).setValue(true)
    expect(last().run_logging?.enabled).toBe(true)
  })

  it('changes the log level on the spot', async () => {
    const { w, onLogLevel } = mountGlobal({}, 'storage')
    await w.findAll('button').find((b) => /^debug$/i.test(b.text()))!.trigger('click')
    expect(onLogLevel).toHaveBeenCalledWith('DEBUG')
  })

  it('asks the page to save on submit', async () => {
    const { w, onSave } = mountGlobal()
    await w.find('form').trigger('submit')
    expect(onSave).toHaveBeenCalledTimes(1)
  })

  describe('reminders', () => {
    const withConnectors = (): Partial<GlobalConfig> => ({
      communication: {
        connectors: {
          'my-telegram': { type: 'telegram', enabled: true, settings: {} },
          'my-slack': { type: 'slack', enabled: true, settings: {} },
          'old-discord': { type: 'discord', enabled: false, settings: {} },
        },
      },
    })

    const toggles = (w: ReturnType<typeof mountGlobal>['w']) =>
      w.find('[data-test="reminder-delivery"]').findAll('input[role="switch"]')
    const toggleFor = (w: ReturnType<typeof mountGlobal>['w'], name: RegExp) =>
      toggles(w).find((t) => name.test(t.element.closest('label')?.textContent ?? ''))!

    it('always includes the web UI and offers each enabled connector type, ticked by default', () => {
      const { w } = mountGlobal(withConnectors())
      const rows = toggles(w).map((t) => [t.element.closest('label')!.textContent!.trim(), (t.element as HTMLInputElement).checked, (t.element as HTMLInputElement).disabled])
      expect(rows).toEqual([
        ['Web UI', true, true],
        ['telegram', true, false],
        ['slack', true, false],
      ])
      expect(w.get('[data-test="reminder-delivery-summary"]').text()).toBe('Reminders go to: Web UI + telegram, slack')
    })

    it('says so when no connector is switched on', () => {
      const { w } = mountGlobal()
      expect(w.get('[data-test="reminder-delivery"]').text()).toMatch(/no connector is switched on/i)
      expect(w.get('[data-test="reminder-delivery-summary"]').text()).toBe('Reminders go to: Web UI')
    })

    it('saves a connector as skipped when it is ticked off, keeping the timezone', async () => {
      const { w, last } = mountGlobal({ ...withConnectors(), reminders: { timezone: 'Europe/London' } })
      await toggleFor(w, /slack/).setValue(false)
      expect(last().reminders).toEqual({ skip_connectors: ['slack'], timezone: 'Europe/London' })
    })

    it('shows a skipped connector as ticked off and ticks it back on', async () => {
      const { w, last } = mountGlobal({ ...withConnectors(), reminders: { skip_connectors: ['slack', 'discord'] } })
      expect((toggleFor(w, /slack/).element as HTMLInputElement).checked).toBe(false)
      expect(w.get('[data-test="reminder-delivery-summary"]').text()).toBe('Reminders go to: Web UI + telegram')
      await toggleFor(w, /slack/).setValue(true)
      // an entry for a connector that is not switched on is kept
      expect(last().reminders?.skip_connectors).toEqual(['discord'])
    })

    it('saves a typed timezone and flags an unknown one', async () => {
      const { w, last } = mountGlobal()
      const tz = fieldByLabel(w, /timezone/i)
      await tz.setValue('Europe/London')
      expect(last().reminders?.timezone).toBe('Europe/London')
      await w.setProps({ editConfig: { ...structuredClone(DEFAULT_CONFIG), reminders: { timezone: 'Mars/Base' } } })
      expect(w.text()).toMatch(/not a known timezone/i)
    })

    it("fills the browser's timezone on request", async () => {
      const { w, last } = mountGlobal()
      const button = w.findAll('button').find((b) => /browser/i.test(b.text()))
      expect(button).toBeDefined()
      await button!.trigger('click')
      expect(last().reminders?.timezone).toBe(Intl.DateTimeFormat().resolvedOptions().timeZone)
    })
  })
})
