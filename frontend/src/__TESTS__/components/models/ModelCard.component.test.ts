import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ModelCard from '../../../components/models/ModelCard.vue'
import type { Model } from '../../../types/model'

const BASE = { name: 'inkling', provider: 'local', filename: 'inkling.gguf', port: 8089, endpoint: '', active: false, ready: false, workload_class: 'local' } as Model

function mountCard(model: Model) {
  return mount(ModelCard, {
    props: { model, providerLabel: 'Local', role: null, canStart: true },
    global: { stubs: { Icon: true } },
  })
}

describe('ModelCard', () => {
  it('says what a small window means and how to lift it', () => {
    const note = mountCard({ ...BASE, small_window: true }).find('[data-test="small-window"]')
    expect(note.exists()).toBe(true)
    expect(note.text()).toContain('core tools')
    expect(note.text()).toContain('--ctx-size 16384')
  })

  it('says nothing for a model with room for every tool', () => {
    expect(mountCard(BASE).find('[data-test="small-window"]').exists()).toBe(false)
  })
})
