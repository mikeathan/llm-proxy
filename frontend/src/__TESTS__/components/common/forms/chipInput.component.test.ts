import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ChipInput from '../../../../components/common/forms/ChipInput.vue'

const suggestions = [
  { value: 'https://bbc.example/rss.xml', label: 'BBC News', note: 'bbc.example' },
  { value: 'https://guardian.example/rss', label: 'The Guardian' },
  { value: 'https://ft.example/rss', label: 'Financial Times' },
]
const isAddress = (v: string): boolean => /^https?:\/\/\S+$/i.test(v)

function setup(extra: Record<string, unknown> = {}) {
  const w = mount(ChipInput, { props: { modelValue: [], label: 'Feeds', suggestions, accept: isAddress, rejected: 'Pick one from the list, or paste an address.', ...extra, 'onUpdate:modelValue': (v: string[]) => w.setProps({ modelValue: v }) } })
  return w
}
const input = (w: ReturnType<typeof setup>) => w.get('input')
const optionTexts = (w: ReturnType<typeof setup>) => w.findAll('[role="option"]').map((o) => o.text())

describe('ChipInput', () => {
  it('lists the suggestions that match what is typed, by label, address or note, and none for an empty box', async () => {
    const w = setup()
    expect(w.find('[role="listbox"]').exists()).toBe(false)
    await input(w).setValue('guard')
    expect(optionTexts(w)).toEqual(['The Guardian'])
    await input(w).setValue('bbc.example')
    expect(optionTexts(w)).toEqual(['BBC Newsbbc.example'])
    expect(input(w).attributes('aria-expanded')).toBe('true')
  })

  it('adds the chosen suggestion\'s value on a click, and does not offer one that is already added', async () => {
    const w = setup()
    await input(w).setValue('news')
    await w.get('[role="option"]').trigger('mousedown')
    expect(w.props('modelValue')).toEqual(['https://bbc.example/rss.xml'])
    await input(w).setValue('bbc')
    expect(w.find('[role="option"]').exists()).toBe(false)
  })

  it('picks with the arrow keys and Enter, and Escape closes the list', async () => {
    const w = setup()
    await input(w).setValue('e')
    await input(w).trigger('keydown', { key: 'ArrowDown' })
    await input(w).trigger('keydown', { key: 'ArrowDown' })
    await input(w).trigger('keydown', { key: 'Enter' })
    expect(w.props('modelValue')).toEqual([optionTextValue(1)])
    await input(w).setValue('f')
    await input(w).trigger('keydown', { key: 'Escape' })
    expect(w.find('[role="listbox"]').exists()).toBe(false)
  })

  it('takes the first suggestion on Enter when what was typed is not an address, and keeps a refused word in the box', async () => {
    const w = setup()
    await input(w).setValue('times')
    await input(w).trigger('keydown', { key: 'Enter' })
    expect(w.props('modelValue')).toEqual(['https://ft.example/rss'])
    await input(w).setValue('nothing matches')
    await input(w).trigger('keydown', { key: 'Enter' })
    expect(w.props('modelValue')).toEqual(['https://ft.example/rss'])
    expect((input(w).element as HTMLInputElement).value).toBe('nothing matches')
    expect(w.get('[role="alert"]').text()).toContain('Pick one from the list')
  })

  it('accepts a pasted address and shows a chip by its label', async () => {
    const w = setup({ labels: { 'https://bbc.example/rss.xml': 'BBC News' } })
    await input(w).setValue('https://other.example/feed.xml')
    await input(w).trigger('keydown', { key: 'Enter' })
    await w.setProps({ modelValue: ['https://bbc.example/rss.xml', 'https://other.example/feed.xml'] })
    const chips = w.findAll('[data-test="chip-input"] > span').map((c) => c.text())
    expect(chips).toEqual(['BBC News×', 'https://other.example/feed.xml×'])
    expect(w.find('button[aria-label="Remove BBC News"]').exists()).toBe(true)
  })

  it('is the plain chip box when it has no suggestions: Enter adds, a repeat is ignored', async () => {
    const w = mount(ChipInput, { props: { modelValue: ['AI'], label: 'Searches' } })
    expect(w.get('input').attributes('role')).toBeUndefined()
    await w.get('input').setValue('ai')
    await w.get('input').trigger('keydown', { key: 'Enter' })
    expect(w.emitted('update:modelValue')).toBeUndefined()
    await w.get('input').setValue('Ukraine')
    await w.get('input').trigger('keydown', { key: 'Enter' })
    expect(w.emitted('update:modelValue')![0]).toEqual([['AI', 'Ukraine']])
  })
})

function optionTextValue(i: number): string {
  return suggestions[i]!.value
}
