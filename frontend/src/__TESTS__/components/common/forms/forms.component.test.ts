import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import SegmentedControl from '../../../../components/common/forms/SegmentedControl.vue'
import SearchInput from '../../../../components/common/forms/SearchInput.vue'
import SelectInput from '../../../../components/common/forms/SelectInput.vue'
import LogViewer from '../../../../components/common/display/LogViewer.vue'
import FormField from '../../../../components/common/forms/FormField.vue'
import ListField from '../../../../components/common/forms/ListField.vue'
import InheritField from '../../../../components/common/forms/InheritField.vue'

const OPTIONS = [{ value: 'runs', label: 'Runs' }, { value: 'app', label: 'App log' }, { value: 'process', label: 'Process log' }]

describe('SegmentedControl', () => {
  it('is a labelled radio group with one checked, tabbable option', () => {
    const w = mount(SegmentedControl, { props: { modelValue: 'app', options: OPTIONS, label: 'Activity view' }, attachTo: document.body })
    expect(w.get('[role="radiogroup"]').attributes('aria-label')).toBe('Activity view')
    const radios = w.findAll('[role="radio"]')
    expect(radios.map((r) => r.attributes('aria-checked'))).toEqual(['false', 'true', 'false'])
    expect(radios.map((r) => r.attributes('tabindex'))).toEqual(['-1', '0', '-1'])
    w.unmount()
  })

  it('selects on click and moves with the arrow keys, wrapping', async () => {
    const w = mount(SegmentedControl, { props: { modelValue: 'runs', options: OPTIONS, label: 'View' }, attachTo: document.body })
    await w.findAll('[role="radio"]')[2]!.trigger('click')
    await w.findAll('[role="radio"]')[0]!.trigger('keydown', { key: 'ArrowLeft' })
    await w.findAll('[role="radio"]')[0]!.trigger('keydown', { key: 'ArrowRight' })
    expect(w.emitted('update:modelValue')).toEqual([['process'], ['process'], ['app']])
    w.unmount()
  })
})

describe('InheritField', () => {
  const radios = (w: ReturnType<typeof mount>) => w.findAll('[role="radio"]')

  it('offers default, on and off, and names the live default in the first option', () => {
    const on = mount(InheritField, { props: { modelValue: '', defaultOn: true, label: 'Memory' } })
    expect(radios(on).map((r) => r.text())).toEqual(['Default (on)', 'On', 'Off'])
    expect(radios(on).map((r) => r.attributes('aria-checked'))).toEqual(['true', 'false', 'false'])
    const off = mount(InheritField, { props: { modelValue: 'off', defaultOn: false, label: 'Memory' } })
    expect(radios(off).map((r) => r.text())).toEqual(['Default (off)', 'On', 'Off'])
    expect(radios(off).map((r) => r.attributes('aria-checked'))).toEqual(['false', 'false', 'true'])
  })

  it('is a labelled radio group that emits inherit as an empty string', async () => {
    const w = mount(InheritField, { props: { modelValue: 'on', defaultOn: false, label: 'Memory' } })
    expect(w.get('[role="radiogroup"]').attributes('aria-label')).toBe('Memory')
    await radios(w)[2]!.trigger('click')
    await radios(w)[0]!.trigger('click')
    expect(w.emitted('update:modelValue')).toEqual([['off'], ['']])
  })
})

describe('SearchInput', () => {
  it('is a labelled search field bound with v-model', async () => {
    const w = mount(SearchInput, { props: { modelValue: '', label: 'Search runs' } })
    const input = w.get('input')
    expect(input.attributes('type')).toBe('search')
    expect(input.attributes('aria-label')).toBe('Search runs')
    await input.setValue('nightly')
    expect(w.emitted('update:modelValue')).toEqual([['nightly']])
  })
})

describe('SelectInput', () => {
  it('is a labelled select bound with v-model', async () => {
    const w = mount(SelectInput, { props: { modelValue: '', options: [{ value: '', label: 'Any status' }, { value: 'failed', label: 'Failed' }], label: 'Status' } })
    expect(w.get('select').attributes('aria-label')).toBe('Status')
    await w.get('select').setValue('failed')
    expect(w.emitted('update:modelValue')).toEqual([['failed']])
  })
})

describe('LogViewer', () => {
  it('is a focusable, labelled region showing the text, or a placeholder when empty', () => {
    const w = mount(LogViewer, { props: { text: 'line one\nline two', label: 'Process log' } })
    const region = w.get('[role="region"]')
    expect(region.attributes('aria-label')).toBe('Process log')
    expect(region.attributes('tabindex')).toBe('0')
    expect(w.text()).toContain('line two')
    expect(mount(LogViewer, { props: { text: '', label: 'Process log', emptyText: 'No process is running.' } }).text()).toContain('No process is running.')
  })
})

describe('FormField', () => {
  const control = '<template #default="{ id, describedBy, invalid }"><input :id="id" :aria-describedby="describedBy" :aria-invalid="invalid ? \'true\' : undefined" /></template>'

  it('labels its control and describes it with the hint', () => {
    const w = mount(FormField, { props: { label: 'Name', hint: 'Lowercase is fine.' }, slots: { default: control } })
    const input = w.get('input')
    expect(w.get('label').attributes('for')).toBe(input.attributes('id'))
    expect(w.get(`#${input.attributes('aria-describedby')}`).text()).toBe('Lowercase is fine.')
  })

  it('replaces the hint with an announced error and marks the control invalid', () => {
    const w = mount(FormField, { props: { label: 'Name', hint: 'x', error: 'Required' }, slots: { default: control } })
    expect(w.get('[role="alert"]').text()).toBe('Required')
    expect(w.get('input').attributes('aria-invalid')).toBe('true')
  })
})

describe('ListField', () => {
  it('reports one entry per line, trimmed, without blanks, keeping what is being typed', async () => {
    const w = mount(ListField, { props: { modelValue: ['ls'] } })
    const area = w.get('textarea')
    expect((area.element as HTMLTextAreaElement).value).toBe('ls')
    await area.setValue('ls\n  git \n\n')
    const emitted = w.emitted('update:modelValue')!
    expect(emitted[emitted.length - 1]).toEqual([['ls', 'git']])
    // The parent echoes the list back: the half-typed trailing lines stay.
    await w.setProps({ modelValue: ['ls', 'git'] })
    expect((area.element as HTMLTextAreaElement).value).toBe('ls\n  git \n\n')
  })

  it('shows a list replaced from outside', async () => {
    const w = mount(ListField, { props: { modelValue: ['ls'] } })
    await w.setProps({ modelValue: ['cat', 'rg'] })
    expect((w.get('textarea').element as HTMLTextAreaElement).value).toBe('cat\nrg')
  })
})
