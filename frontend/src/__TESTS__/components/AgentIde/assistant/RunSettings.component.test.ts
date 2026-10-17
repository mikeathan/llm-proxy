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
})
