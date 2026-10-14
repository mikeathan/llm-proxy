import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import GuardrailForm from '../../../../components/AgentIde/system/GuardrailForm.vue'
import { DEFAULT_CONFIG } from '../../../../composables/models/useConfig'
import { fieldByLabel } from '../../../helpers/fieldByLabel'
import type { AgentGuardrailsConfig } from '../../../../types/admin'
import { seedLayer } from '../../../../domain/guardrailLayers'

// Characterised before the Settings (workspace) redesign (plan D22). The form
// edits a whole policy and reports every change as update:modelValue.
function mountForm(policy: AgentGuardrailsConfig = structuredClone(DEFAULT_CONFIG.guardrails)) {
  const onUpdate = vi.fn()
  const w = mount(GuardrailForm, {
    props: { modelValue: policy },
    attrs: { 'onUpdate:modelValue': onUpdate },
    global: { stubs: { Icon: true } },
  })
  const last = (): AgentGuardrailsConfig => onUpdate.mock.lastCall![0]
  return { w, onUpdate, last }
}

const ENABLED: AgentGuardrailsConfig = {
  ...structuredClone(DEFAULT_CONFIG.guardrails),
  terminal: { ...DEFAULT_CONFIG.guardrails.terminal, enabled: true, allowed_commands: ['ls'] },
}

describe('GuardrailForm', () => {
  it('stacks its sections in one column instead of an xl two-column grid', () => {
    const { w } = mountForm()
    const root = w.element.className
    expect(root).not.toMatch(/grid-cols-2/)
    expect(w.findAll('section').length).toBeGreaterThan(1)
  })

  describe('folding', () => {
    const toggles = (w: ReturnType<typeof mountForm>['w']) => w.findAll('button[aria-expanded]')
    const toggle = (w: ReturnType<typeof mountForm>['w'], title: RegExp) => toggles(w).find((b) => title.test(b.text()))!

    it('starts with every section folded to a line that says what it does', () => {
      const { w } = mountForm(structuredClone(ENABLED))
      expect(toggles(w).length).toBeGreaterThan(1)
      expect(toggles(w).every((b) => b.attributes('aria-expanded') === 'false')).toBe(true)
      expect(toggle(w, /terminal tool/i).text()).toContain('1 allowed command')
      expect(toggle(w, /secrets and patterns/i).text()).toMatch(/blocking secrets/i)
    })

    it('opens a section on click and folds it again', async () => {
      const { w } = mountForm(structuredClone(ENABLED))
      await toggle(w, /terminal tool/i).trigger('click')
      expect(toggle(w, /terminal tool/i).attributes('aria-expanded')).toBe('true')
      await toggle(w, /terminal tool/i).trigger('click')
      expect(toggle(w, /terminal tool/i).attributes('aria-expanded')).toBe('false')
    })

    it('has nothing to open in a section whose tool is switched off', () => {
      const { w } = mountForm()
      expect(toggles(w).some((b) => /terminal tool/i.test(b.text()))).toBe(false)
      expect(w.text()).toMatch(/terminal tool/i)
    })

    it('opens a section as soon as its tool is switched on', async () => {
      const { w } = mountForm()
      await fieldByLabel(w, /^communication$/i).setValue(true)
      expect(toggle(w, /communication/i).attributes('aria-expanded')).toBe('true')
    })
  })

  it('enables communication and edits approval and message limits', async () => {
    const { w, last } = mountForm()
    await fieldByLabel(w, /^communication$/i).setValue(true)
    expect(last().communication.enabled).toBe(true)
    await fieldByLabel(w, /approve each notification/i).setValue(false)
    await fieldByLabel(w, /max messages per task/i).setValue('3')
    expect(last().communication).toEqual({ enabled: true, require_review: false, max_messages_per_task: 3 })
  })
  it('reports a switched setting', async () => {
    const { w, last } = mountForm()
    await fieldByLabel(w, /block secrets/i).setValue(false)
    expect(last().global.block_secrets).toBe(false)
  })

  it('reports an edited number', async () => {
    const { w, last } = mountForm(structuredClone(ENABLED))
    await fieldByLabel(w, /^command timeout \(sec\)$/i).setValue('45')
    expect(last().terminal.timeout_seconds).toBe(45)
  })

  it('reports an edited list, one entry per line', async () => {
    const { w, last } = mountForm(structuredClone(ENABLED))
    await fieldByLabel(w, /allowed commands/i).setValue('ls\ngit\n')
    expect(last().terminal.allowed_commands).toEqual(['ls', 'git'])
  })

  it('shows a tool section only while the tool is enabled', () => {
    expect(mountForm().w.text()).not.toMatch(/allowed commands/i)
    expect(mountForm(structuredClone(ENABLED)).w.text()).toMatch(/allowed commands/i)
  })
})

// Layer mode: the form edits a workspace's own layer over the global policy.
function mountLayer(global: AgentGuardrailsConfig = structuredClone(ENABLED)) {
  const onUpdate = vi.fn()
  const w = mount(GuardrailForm, {
    props: { modelValue: seedLayer(global), inherited: global },
    attrs: { 'onUpdate:modelValue': onUpdate },
    global: { stubs: { Icon: true } },
  })
  const last = (): AgentGuardrailsConfig => onUpdate.mock.lastCall![0]
  return { w, last }
}

describe('GuardrailForm over the global policy', () => {
  it('enables communication just for the workspace and can waive inherited approval', async () => {
    const { w, last } = mountLayer()
    await fieldByLabel(w, /^communication$/i).setValue(true)
    expect(last().communication.enabled).toBe(true)
    await fieldByLabel(w, /approve each notification/i).setValue(false)
    await w.setProps({ modelValue: last() })
    expect(w.get('[data-test="source-communication-require_review"]').text()).toBe('Exception')
  })

  it('shows globally waived communication approval as off and prevents re-enabling it in a workspace', () => {
    const global = structuredClone(ENABLED)
    global.communication = { enabled: true, require_review: false, max_messages_per_task: 5 }
    const { w } = mountLayer(global)
    const enabled = fieldByLabel(w, /^communication$/i)
    expect((enabled.element as HTMLInputElement).checked).toBe(true)
    expect(enabled.attributes('disabled')).toBeDefined()
    const review = fieldByLabel(w, /approve each notification/i)
    expect((review.element as HTMLInputElement).checked).toBe(false)
    expect(review.attributes('disabled')).toBeDefined()
  })
  it('shows global list entries as fixed, and edits only what the workspace adds', async () => {
    const { w, last } = mountLayer()
    const field = fieldByLabel(w, /allowed commands/i)
    expect((field.element as HTMLTextAreaElement).value).toBe('')
    expect(w.get('[data-test="inherited-terminal-allowed_commands"]').text()).toContain('ls')
    await field.setValue('git')
    expect(last().terminal.allowed_commands).toEqual(['git'])
    await w.setProps({ modelValue: last() })
    expect(w.get('[data-test="source-terminal-allowed_commands"]').text()).toBe('Exception')
  })

  it('cannot switch off what the global policy switches on', () => {
    const { w } = mountLayer()
    const terminal = fieldByLabel(w, /^terminal tool$/i)
    expect((terminal.element as HTMLInputElement).checked).toBe(true)
    expect(terminal.attributes('disabled')).toBeDefined()
  })

  it('leaves a number empty to inherit it, and overrides it when typed', async () => {
    const { w, last } = mountLayer()
    const timeout = fieldByLabel(w, /^command timeout \(sec\)$/i)
    expect((timeout.element as HTMLInputElement).value).toBe('')
    expect(timeout.attributes('placeholder')).toContain(String(ENABLED.terminal.timeout_seconds))
    await timeout.setValue('90')
    expect(last().terminal.timeout_seconds).toBe(90)
    await timeout.setValue('')
    expect(last().terminal.timeout_seconds).toBe(0)
  })

  it('opens the sections the workspace changes, and folds the ones it inherits', () => {
    const global = structuredClone(ENABLED)
    const layer = seedLayer(global)
    layer.terminal.allowed_commands = ['git']
    const w = mount(GuardrailForm, { props: { modelValue: layer, inherited: global }, global: { stubs: { Icon: true } } })
    const state = (title: RegExp) => w.findAll('button[aria-expanded]').find((b) => title.test(b.text()))?.attributes('aria-expanded')
    expect(state(/terminal tool/i)).toBe('true')
    expect(state(/secrets and patterns/i)).toBe('false')
    expect(state(/communication/i)).toBeUndefined() // off globally: nothing to open
  })

  it('says how many changes a folded workspace section holds', async () => {
    const global = structuredClone(ENABLED)
    global.network.enabled = true
    const layer = seedLayer(global)
    layer.network.allow_lan_access = true
    const w = mount(GuardrailForm, { props: { modelValue: layer, inherited: global }, global: { stubs: { Icon: true } } })
    const network = w.findAll('section').find((x) => /agent network access/i.test(x.text()))!
    expect(network.find('[data-test="changes-network"]').exists()).toBe(false) // open: the fields carry their own tags
    await network.get('button').trigger('click')
    expect(network.get('[data-test="changes-network"]').text()).toMatch(/1 change/)
  })

  it('labels every field only when editing a workspace layer', () => {
    expect(mountForm(structuredClone(ENABLED)).w.find('[data-test^="source-"]').exists()).toBe(false)
    expect(mountLayer().w.get('[data-test="source-global-block_secrets"]').text()).toBe('Inherited')
  })

  it('points out entries the global policy already has', async () => {
    const { w, last } = mountLayer()
    await fieldByLabel(w, /allowed commands/i).setValue('ls\ngit')
    await w.setProps({ modelValue: last() })
    expect(w.get('[data-test="redundant-terminal-allowed_commands"]').text()).toContain('ls')
    expect(w.get('[data-test="redundant-terminal-allowed_commands"]').text()).not.toContain('git')
  })
})
