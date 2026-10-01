import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ThemeEditor from '../../../components/settings/ThemeEditor.vue'
import { EDITABLE_TOKENS, draftFromPreset } from '../../../theme/themeDraft'
import { fieldByLabel } from '../../helpers/fieldByLabel'
import type { ThemeDraft } from '../../../types/theme'

// The page owns saving (useTheme); the editor's contract is what it emits.
function mountEditor(initial: ThemeDraft = draftFromPreset('retro-dark', 'Sea', []), isNew = true) {
  const onSave = vi.fn()
  const onCancel = vi.fn()
  const onDirty = vi.fn()
  const w = mount(ThemeEditor, {
    props: { initial, isNew, takenIds: ['amber'] },
    attrs: { onSave, onCancel, onDirtyChange: onDirty },
    global: { stubs: { Icon: true } },
  })
  const save = () => w.findAll('button').find((b) => /^save theme$/i.test(b.text()))!
  return { w, onSave, onCancel, onDirty, save }
}

describe('ThemeEditor', () => {
  it('has one field per editable token, starting from the base preset', () => {
    const { w } = mountEditor()
    expect(w.findAll('input[data-token]')).toHaveLength(EDITABLE_TOKENS.length)
    expect((fieldByLabel(w, /^--canvas$/).element as HTMLInputElement).value).toMatch(/^#[0-9a-f]{6}$/)
  })

  it('saves a new theme with only the changed tokens, named after its label', async () => {
    const { w, onSave, save } = mountEditor()
    await fieldByLabel(w, /^name$/i).setValue('Deep sea')
    await fieldByLabel(w, /^--accent-brand$/).setValue('#ffa07a')
    await save().trigger('click')
    expect(onSave).toHaveBeenCalledWith({
      input: { id: 'deep-sea', label: 'Deep sea', base: 'retro-dark', overrides: { 'accent-brand': '#ffa07a' } },
      acknowledgeContrast: false,
    })
  })

  it('keeps the id of an existing theme when it is renamed', async () => {
    const existing = { ...draftFromPreset('retro-dark', 'Amber', []), id: 'amber' }
    const { w, onSave, save, onDirty } = mountEditor(existing, false)
    expect(save().attributes('disabled')).toBeDefined()
    await fieldByLabel(w, /^name$/i).setValue('Amber glow')
    expect(onDirty).toHaveBeenLastCalledWith(true)
    await save().trigger('click')
    expect(onSave.mock.lastCall![0].input).toMatchObject({ id: 'amber', label: 'Amber glow' })
  })

  it('rejects a value that is not valid for its token, and never previews it', async () => {
    const { w, save } = mountEditor()
    await fieldByLabel(w, /^--canvas$/).setValue('url(https://example.com/x.png)')
    expect(w.text()).toContain('is not a valid colour')
    expect(fieldByLabel(w, /^--canvas$/).attributes('aria-invalid')).toBe('true')
    expect(save().attributes('disabled')).toBeDefined()
    expect(w.get('[data-test="theme-preview"]').attributes('style') ?? '').not.toContain('url(')
  })

  it('lists failing contrast pairs and saves only once they are acknowledged', async () => {
    const { w, onSave, save } = mountEditor()
    await fieldByLabel(w, /^--text-muted$/).setValue('#1a1a1a')
    const warning = w.get('[data-test="contrast-warning"]')
    expect(warning.text()).toMatch(/--text-muted on --canvas/)
    expect(warning.text()).toMatch(/4\.5:1/)
    expect(save().attributes('disabled')).toBeDefined()
    await fieldByLabel(w, /save anyway/i).setValue(true)
    await save().trigger('click')
    expect(onSave.mock.lastCall![0].acknowledgeContrast).toBe(true)
  })

  it('previews edits on a scoped sample, not the page', async () => {
    const { w } = mountEditor()
    await fieldByLabel(w, /^--accent-brand$/).setValue('#ff0000')
    const preview = w.get('[data-test="theme-preview"]')
    expect(preview.attributes('data-theme')).toBe('retro-dark')
    expect(preview.attributes('style')).toContain('--accent-brand: 255 0 0')
    expect(document.documentElement.style.getPropertyValue('--accent-brand')).toBe('')
  })

  it('cancels on request', async () => {
    const { w, onCancel } = mountEditor()
    await w.findAll('button').find((b) => /^cancel$/i.test(b.text()))!.trigger('click')
    expect(onCancel).toHaveBeenCalledTimes(1)
  })
})
