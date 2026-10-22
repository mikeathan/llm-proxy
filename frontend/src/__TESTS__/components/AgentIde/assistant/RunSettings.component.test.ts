import { describe, it, expect, afterEach } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import RunSettings from '../../../../components/AgentIde/assistant/RunSettings.vue'

const PROFILE_OPTIONS = [{ value: 'general', label: 'General (default)' }, { value: 'research', label: 'Research' }]
const MODEL_OPTIONS = [{ value: '', label: 'Default (a)' }, { value: 'b', label: 'b' }]

const mounted: VueWrapper[] = []
function mountSettings(over: Record<string, unknown> = {}) {
  const w = mount(RunSettings, {
    props: { profile: 'general', model: '', profileOptions: PROFILE_OPTIONS, modelOptions: MODEL_OPTIONS, profileName: 'General', profileSummary: 'Everyday assistant.', ...over },
    global: { stubs: { Icon: true } },
    attachTo: document.body,
  })
  mounted.push(w)
  return w
}
afterEach(() => {
  mounted.splice(0).forEach((w) => w.unmount())
  document.body.innerHTML = ''
})

describe('RunSettings', () => {
  it('is one button naming the profile, with the full choice in its label', () => {
    const button = mountSettings({ model: 'b' }).get('[data-test="run-settings"]')
    expect(button.text()).toBe('General')
    expect(button.attributes('aria-label')).toBe('Run settings: General profile, b. Change')
    expect(button.attributes('aria-expanded')).toBe('false')
  })

  it('opens the choices above the button and closes on Escape', async () => {
    const w = mountSettings()
    expect(w.find('[role="dialog"]').exists()).toBe(false)
    await w.get('[data-test="run-settings"]').trigger('click')
    const dialog = w.get('[role="dialog"]')
    expect(dialog.classes()).toContain('sm:bottom-full')
    expect(w.get('[data-test="run-settings"]').attributes('aria-expanded')).toBe('true')
    expect(dialog.text()).toContain('Everyday assistant.')
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await w.vm.$nextTick()
    expect(w.find('[role="dialog"]').exists()).toBe(false)
  })

  it('opens as a sheet on <body>, above the tab bar, and a press inside it does not dismiss it', async () => {
    const w = mountSettings({ sheet: true })
    await w.get('[data-test="run-settings"]').trigger('click')
    expect(w.find('[role="dialog"]').exists()).toBe(false)
    const dialog = document.body.querySelector('[role="dialog"]') as HTMLElement
    expect(dialog).not.toBeNull()
    expect(dialog.parentElement?.className).toContain('z-50')
    dialog.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }))
    await w.vm.$nextTick()
    expect(document.body.querySelector('[role="dialog"]')).not.toBeNull()
    document.body.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }))
    await w.vm.$nextTick()
    expect(document.body.querySelector('[role="dialog"]')).toBeNull()
  })

  it('reports a profile pick and a model pick', async () => {
    const w = mountSettings()
    await w.get('[data-test="run-settings"]').trigger('click')
    await w.get('select[aria-label="Profile"]').setValue('research')
    await w.get('select[aria-label="Model"]').setValue('b')
    expect(w.emitted('update:profile')?.[0]).toEqual(['research'])
    expect(w.emitted('update:model')?.[0]).toEqual(['b'])
  })

  it('offers no model choice when there is only the default', async () => {
    const w = mountSettings({ modelOptions: [{ value: '', label: 'Default (a)' }] })
    await w.get('[data-test="run-settings"]').trigger('click')
    expect(w.find('select[aria-label="Model"]').exists()).toBe(false)
    expect(w.find('select[aria-label="Profile"]').exists()).toBe(true)
  })

  it('cannot be opened while a run is going', () => {
    expect(mountSettings({ disabled: true }).get('[data-test="run-settings"]').attributes('disabled')).toBeDefined()
  })

  // The "Use helpers" row: shown when the profile names helpers, enabled when the server says they can run.
  describe('helpers', () => {
    const HELPERS = { helpersShown: true, helpersAvailable: true, helpersModel: 'deepseek-v4.1-flash' }
    const openIt = async (over: Record<string, unknown>) => {
      const w = mountSettings({ profile: 'research', ...over })
      await w.get('[data-test="run-settings"]').trigger('click')
      return w
    }

    it('has no helpers row for a profile that names no helpers', async () => {
      const w = await openIt({ helpersShown: false })
      expect(w.text()).not.toContain('Use helpers')
    })

    it('offers the switch, names the helper model and what leaves the machine, and reports a toggle', async () => {
      const w = await openIt(HELPERS)
      const toggle = w.get('input[type="checkbox"]')
      expect((toggle.element as HTMLInputElement).disabled).toBe(false)
      expect(w.text()).toContain('deepseek-v4.1-flash')
      expect(w.text()).toContain('leaves this machine')
      await toggle.setValue(true)
      expect(w.emitted('update:helpers')?.[0]).toEqual([true])
    })

    it('shows the switch disabled, with the reason, when no helper model is set up', async () => {
      const w = await openIt({ ...HELPERS, helpersAvailable: false, helpersModel: '' })
      expect((w.get('input[type="checkbox"]').element as HTMLInputElement).disabled).toBe(true)
      expect(w.text()).toContain('Settings → Runs')
    })

    it('says helpers are on in the button, and only when they will be used', () => {
      expect(mountSettings({ ...HELPERS, helpers: true }).get('[data-test="run-settings"]').text()).toContain('helpers')
      expect(mountSettings({ ...HELPERS, helpers: false }).get('[data-test="run-settings"]').text()).not.toContain('helpers')
      expect(mountSettings({ ...HELPERS, helpers: true, helpersAvailable: false }).get('[data-test="run-settings"]').text()).not.toContain('helpers')
      expect(mountSettings({ ...HELPERS, helpers: true }).get('[data-test="run-settings"]').attributes('aria-label')).toContain('helpers on')
    })
  })
})
