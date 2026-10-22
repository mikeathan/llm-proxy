import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import GlobalSettings from '../../../components/settings/GlobalSettings.vue'
import { DEFAULT_CONFIG } from '../../../composables/models/useConfig'
import { fieldByLabel } from '../../helpers/fieldByLabel'
import type { GlobalConfig, SettingsTab } from '../../../types/admin'
import type { Model } from '../../../types/model'

const MODELS = [{ name: 'qwen', provider: 'local' }, { name: 'gpt', provider: 'openai' }] as Model[]

// VTU's emitted() does not record script-setup emits in this harness, so the
// listener callbacks are the component's observable contract.
function mountGlobal(overrides: Partial<GlobalConfig> = {}, section: SettingsTab = 'local') {
  const onUpdate = vi.fn()
  const onSave = vi.fn()
  const onLogLevel = vi.fn()
  const w = mount(GlobalSettings, {
    props: { editConfig: { ...structuredClone(DEFAULT_CONFIG), ...overrides }, logLevel: 'info', models: MODELS, section },
    attrs: { 'onUpdate:editConfig': onUpdate, onUpdateConfig: onSave, onUpdateLogLevel: onLogLevel },
    global: { stubs: { Icon: true } },
  })
  const last = (): GlobalConfig => onUpdate.mock.lastCall![0]
  return { w, onUpdate, onSave, onLogLevel, last }
}

const SCHEDULER_DEFAULTS = {
  local_concurrency: 1,
  cloud_concurrency: 3,
  preempt_automations: true,
  inbound_wait_seconds: 60,
  inbound_wait_by_default: false,
  inbound_max_queued: 32,
  inbound_preempt: false,
}

// General is four short sections (plan frontend-redesign-v2): each shows only its own panels, all stay mounted.
describe('GlobalSettings sections', () => {
  const visiblePanels = (w: ReturnType<typeof mountGlobal>['w']) =>
    w.findAll('section').filter((p) => !p.attributes('style')?.includes('display: none')).map((p) => p.get('h2').text().replace(/^\d+\s*—?\s*/, '').toLowerCase())

  it.each([
    ['local', ['routing', 'local engine', 'model lifecycle', 'gpu metrics']],
    ['runs', ['run scheduler', 'external requests']],
    ['memory', ['memory', 'reminders']],
    ['storage', ['workspace storage', 'logging']],
  ] as const)('%s shows only its panels', (section, titles) => {
    expect(visiblePanels(mountGlobal({}, section).w)).toEqual(titles)
  })

  it('keeps every panel in the page, so an edit survives moving between sections', () => {
    const { w } = mountGlobal({}, 'runs')
    expect(w.findAll('section')).toHaveLength(10)
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

// A tool result (a file, a page, search hits) keeps more text when the window has room, and never less than 3000 characters:
// the switch is on unless the saved config says otherwise, and turning it off is saved as an explicit false.
describe('GlobalSettings tool result size', () => {
  const toggle = (w: ReturnType<typeof mountGlobal>['w']) =>
    w.findAll('input[type="checkbox"]').find((t) => /tool results/i.test(t.element.closest('label')?.textContent ?? ''))!

  it('is on when the config says nothing', () => {
    const { w } = mountGlobal({}, 'runs')
    expect((toggle(w).element as HTMLInputElement).checked).toBe(true)
    expect(w.text()).toContain('never less than 3000 characters')
  })

  it('reflects a saved off', () => {
    const { w } = mountGlobal({ agent: { adaptive_result_cap: false } }, 'runs')
    expect((toggle(w).element as HTMLInputElement).checked).toBe(false)
  })

  it('saves an explicit false and an explicit true', async () => {
    const { w, last } = mountGlobal({}, 'runs')
    await toggle(w).setValue(false)
    expect(last().agent).toEqual({ adaptive_result_cap: false })
    await toggle(w).setValue(true)
    expect(last().agent).toEqual({ adaptive_result_cap: true })
  })
})

describe('GlobalSettings', () => {
  it('offers every configured model for primary and fallback routing', () => {
    const { w } = mountGlobal()
    const primary = fieldByLabel(w, /primary/i)
    expect(primary.findAll('option').map((o) => o.attributes('value'))).toEqual(['', 'qwen', 'gpt'])
    expect(fieldByLabel(w, /fallback/i).findAll('option')).toHaveLength(3)
  })

  it('writes the scheduler whole, seeded with defaults, and clamps counts to at least one', async () => {
    const { w, last } = mountGlobal()
    await fieldByLabel(w, /local runs/i).setValue('0')
    expect(last().scheduler).toEqual({ ...SCHEDULER_DEFAULTS, local_concurrency: 1 })
    await fieldByLabel(w, /cloud runs/i).setValue('5')
    expect(last().scheduler?.cloud_concurrency).toBe(5)
  })

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

  it('clamps the inbound wait at -1 (no limit)', async () => {
    const { w, last } = mountGlobal()
    await fieldByLabel(w, /wait up to/i).setValue('-5')
    expect(last().scheduler?.inbound_wait_seconds).toBe(-1)
  })

  it('splits default arguments into an argument list', async () => {
    const { w, last } = mountGlobal()
    await fieldByLabel(w, /default arguments/i).setValue('--ctx-size 4096 --flash-attn')
    expect(last().providers?.local?.default_args).toEqual(['--ctx-size', '4096', '--flash-attn'])
  })

  it('commits environment variables on blur, not on every keystroke', async () => {
    const { w, onUpdate, last } = mountGlobal()
    const env = fieldByLabel(w, /environment variables/i)
    await env.setValue('A=1\nB=2')
    expect(onUpdate).not.toHaveBeenCalled()
    await env.trigger('blur')
    expect(last().providers?.local?.environment).toEqual({ A: '1', B: '2' })
  })

  it('shows only the GPU fields the chosen provider uses', () => {
    const labels = (gpu: string) => mountGlobal({ gpu_provider: gpu }).w.findAll('label').map((l) => l.text()).join('|')
    expect(labels('macos')).not.toMatch(/gpu index/i)
    expect(labels('nvidia')).toMatch(/gpu index/i)
    expect(labels('nvidia')).toMatch(/tool binary/i)
    expect(labels('sysfs')).toMatch(/sysfs device path/i)
    expect(labels('sysfs')).not.toMatch(/tool binary/i)
  })

  it('turns run logging on', async () => {
    const { w, last } = mountGlobal({ run_logging: { enabled: false } })
    await fieldByLabel(w, /run logging/i).setValue(true)
    expect(last().run_logging?.enabled).toBe(true)
  })

  it('changes the log level on the spot', async () => {
    const { w, onLogLevel } = mountGlobal()
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

// Helpers (sub-agents) run on a cloud model only, so the picker offers cloud models and "Off"; the pool size rides
// along in the same agent block, keeping the block's other settings.
describe('GlobalSettings helpers', () => {
  const WITH_CLASSES = [
    { name: 'qwen', provider: 'local', workload_class: 'local' },
    { name: 'gpt', provider: 'openai', workload_class: 'cloud' },
  ] as Model[]
  const mountHelpers = (overrides: Partial<GlobalConfig> = {}) => {
    const onUpdate = vi.fn()
    const w = mount(GlobalSettings, {
      props: { editConfig: { ...structuredClone(DEFAULT_CONFIG), ...overrides }, logLevel: 'info', models: WITH_CLASSES, section: 'runs' as SettingsTab },
      attrs: { 'onUpdate:editConfig': onUpdate },
      global: { stubs: { Icon: true } },
    })
    return { w, last: (): GlobalConfig => onUpdate.mock.lastCall![0] }
  }

  it('offers only cloud models, and off by default', () => {
    const { w } = mountHelpers()
    const select = fieldByLabel(w, /Helper model/)
    const options = select.findAll('option').map((o) => o.text())
    expect(options.some((o) => o.startsWith('gpt'))).toBe(true)
    expect(options.some((o) => o.startsWith('qwen'))).toBe(false)
    expect((select.element as HTMLSelectElement).value).toBe('')
  })

  // A Helper model removed from the models (or one that is not a cloud model) still shows, flagged, so the user sees
  // what is set and what to change, not a blank box.
  it('shows a helper model that is no longer available, flagged, and keeps it selected', () => {
    const { w } = mountHelpers({ agent: { subagent_model: 'gone-m' } })
    const select = fieldByLabel(w, /Helper model/)
    const options = select.findAll('option').map((o) => o.text())
    expect(options).toContain('gone-m (not available)')
    expect((select.element as HTMLSelectElement).value).toBe('gone-m')
  })

  it('shows no flagged option when the helper model is available or off', () => {
    expect(fieldByLabel(mountHelpers({ agent: { subagent_model: 'gpt' } }).w, /Helper model/).findAll('option').map((o) => o.text()).join('|')).not.toContain('not available')
    expect(fieldByLabel(mountHelpers().w, /Helper model/).findAll('option').map((o) => o.text()).join('|')).not.toContain('not available')
  })

  it('saves the helper model and pool size beside the other agent settings', async () => {
    const { w, last } = mountHelpers({ agent: { adaptive_result_cap: false } })
    await fieldByLabel(w, /Helper model/).setValue('gpt')
    expect(last().agent).toEqual({ adaptive_result_cap: false, subagent_model: 'gpt' })
    await fieldByLabel(w, /Helpers at once/).setValue('2')
    expect(last().agent?.subagent_concurrency).toBe(2)
  })
})
