import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { ref } from 'vue'
import McpServerForm from '../../../components/agents/McpServerForm.vue'
import { blankMcpForm } from '../../../constants/mcp'
import type { McpServer, McpServerForm as Form } from '../../../types/mcp'

const saved = (over: Partial<McpServer> = {}): McpServer => ({
  name: 'files', url: 'https://a.example.com/mcp', enabled: true, trust: 'ask', auth_kind: 'bearer', has_secret: true, status: 'ready', tools: [], disabled_tools: [], ...over,
})

// A parent that keeps the form, as the page does, so typing is reflected back.
function mountForm(props: { form?: Partial<Form>; current?: McpServer; test?: McpServer; error?: string } = {}) {
  const model = ref<Form>({ ...blankMcpForm(), ...props.form })
  const w = mount(McpServerForm, {
    props: {
      modelValue: model.value, current: props.current, saving: false, testing: false, test: props.test, error: props.error ?? '',
      'onUpdate:modelValue': (f: Form) => { model.value = f; void w.setProps({ modelValue: f }) },
    },
    global: { stubs: { Icon: true } },
  })
  return { w, model }
}

describe('McpServerForm', () => {
  it('cannot be saved until it has a name and an address', async () => {
    const { w } = mountForm()
    expect(w.get('[data-test="mcp-save"]').attributes('disabled')).toBeDefined()
    await w.get('[data-test="mcp-name"]').setValue('files')
    await w.get('[data-test="mcp-url"]').setValue('https://x/mcp')
    expect(w.get('[data-test="mcp-save"]').attributes('disabled')).toBeUndefined()
    await w.get('form').trigger('submit')
    expect(w.emitted('save')).toHaveLength(1)
  })

  it('asks for a header name and a value only for a sign-in that needs them, and holds a half-set sign-in back', async () => {
    const { w } = mountForm({ form: { name: 'files', url: 'https://x/mcp' } })
    expect(w.find('[data-test="mcp-secret"]').exists()).toBe(false)
    await w.findAll('[role="radio"]').find((r) => r.text() === 'Bearer token')!.trigger('click')
    expect(w.find('[data-test="mcp-secret"]').exists()).toBe(true)
    expect(w.find('[data-test="mcp-header"]').exists()).toBe(false)
    expect(w.get('[data-test="mcp-save"]').attributes('disabled')).toBeDefined()
    await w.get('[data-test="mcp-secret"]').setValue('tok')
    expect(w.get('[data-test="mcp-save"]').attributes('disabled')).toBeUndefined()
    await w.findAll('[role="radio"]').find((r) => r.text() === 'A header')!.trigger('click')
    expect(w.find('[data-test="mcp-header"]').exists()).toBe(true)
    expect(w.get('[data-test="mcp-save"]').attributes('disabled')).toBeDefined()
    await w.get('[data-test="mcp-header"]').setValue('X-Api-Key')
    expect(w.get('[data-test="mcp-save"]').attributes('disabled')).toBeUndefined()
  })

  it('types the value as a password and never fills it in', () => {
    const { w } = mountForm({ form: { name: 'files', url: 'https://a.example.com/mcp', auth_kind: 'bearer' }, current: saved() })
    const input = w.get('[data-test="mcp-secret"]')
    expect(input.attributes('type')).toBe('password')
    expect((input.element as HTMLInputElement).value).toBe('')
    expect(input.attributes('placeholder')).toContain('leave empty to keep')
  })

  it('on an edit keeps the stored value when left empty, but asks again for a new address', async () => {
    const { w } = mountForm({ form: { name: 'files', url: 'https://a.example.com/mcp', auth_kind: 'bearer' }, current: saved() })
    expect(w.get('[data-test="mcp-save"]').attributes('disabled')).toBeUndefined()
    await w.get('[data-test="mcp-url"]').setValue('https://a.example.com/other')
    expect(w.get('[data-test="mcp-save"]').attributes('disabled')).toBeUndefined()
    await w.get('[data-test="mcp-url"]').setValue('https://b.example.com/mcp')
    expect(w.get('[data-test="mcp-save"]').attributes('disabled')).toBeDefined()
    expect(w.text()).toContain('never sent to another address')
    await w.get('[data-test="mcp-secret"]').setValue('new')
    expect(w.get('[data-test="mcp-save"]').attributes('disabled')).toBeUndefined()
  })

  it('does not let the name of a saved server be changed', () => {
    const { w } = mountForm({ form: { name: 'files', url: 'https://x/mcp' }, current: saved({ auth_kind: 'none', has_secret: false }) })
    expect(w.get('[data-test="mcp-name"]').attributes('disabled')).toBeDefined()
    expect(w.get('[data-test="mcp-save"]').text()).toBe('Save changes')
  })

  it('says what each trust choice means', async () => {
    const { w } = mountForm({ form: { name: 'files', url: 'https://x/mcp' } })
    expect(w.get('[data-test="mcp-trust-hint"]').text()).toContain('waits for your approval')
    await w.findAll('[role="radio"]').find((r) => r.text() === 'Allow')!.trigger('click')
    expect(w.get('[data-test="mcp-trust-hint"]').text()).toContain('only for a server you trust')
  })

  it('shows the server\'s refusal and a test with its advice', () => {
    const { w } = mountForm({
      form: { name: 'files', url: 'https://x/mcp' }, error: 'a server with that name is already listed',
      test: saved({ status: 'stopped', problem: 'the server\'s certificate is not trusted', problem_code: 'tls', tools: [] }),
    })
    expect(w.get('[data-test="mcp-form-error"]').text()).toContain('already listed')
    expect(w.get('[data-test="mcp-form-test"]').text()).toContain('give its file under Advanced')
  })

  it('tests the entry as typed, and cancels', async () => {
    const { w } = mountForm({ form: { name: 'files', url: 'https://x/mcp' } })
    await w.get('[data-test="mcp-test-form"]').trigger('click')
    await w.get('[data-test="mcp-cancel"]').trigger('click')
    expect(w.emitted('test')).toHaveLength(1)
    expect(w.emitted('cancel')).toHaveLength(1)
  })

  it('opens Advanced when a certificate authority file is set', () => {
    const open = mountForm({ form: { name: 'files', url: 'https://x/mcp', ca_cert: '/etc/ca.pem' } }).w
    expect(open.get('details').attributes('open')).toBeDefined()
    expect(mountForm({ form: { name: 'files', url: 'https://x/mcp' } }).w.get('details').attributes('open')).toBeUndefined()
  })
})
