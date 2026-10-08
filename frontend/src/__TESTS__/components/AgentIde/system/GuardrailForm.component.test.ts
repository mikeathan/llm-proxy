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
