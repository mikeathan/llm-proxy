import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import NewsroomWatchEditor from '../../../components/newsroom/NewsroomWatchEditor.vue'
import type { NewsroomWatch } from '../../../types/newsroom'

const watch = (instance?: string): NewsroomWatch => ({
  id: 1, name: 'Economy', interval: '1h0m0s', max_age: '72h0m0s', enabled: true,
  sources: [{ kind: 'newssearch', target: 'inflation', ...(instance === undefined ? {} : { instance }) }],
})
const mountEditor = (w: NewsroomWatch) => mount(NewsroomWatchEditor, { props: { watch: w, channels: [], saving: false, error: '' }, global: { stubs: { Icon: true } } })

describe('NewsroomWatchEditor, what the watch learned', () => {
  it('lists the posts the owner judged, removes one marked by mistake, and saves the rest', async () => {
    const w = mountEditor({ ...watch(), examples: { want: ['Qwen4 released'], skip: ['Meme: AGI pilled', 'Promo credits'] } })
    const block = w.get('[data-test="watch-examples"]')
    expect(block.text()).toContain('Qwen4 released')
    expect(block.text()).toContain('Meme: AGI pilled')
    await w.get('[aria-label="Remove Meme: AGI pilled"]').trigger('click')
    await w.get('form').trigger('submit')
    expect(w.emitted('save')![0]![0]).toMatchObject({ examples: { want: ['Qwen4 released'], skip: ['Promo credits'] } })
  })

  it('says nothing about examples until the owner has judged a post', () => {
    expect(mountEditor(watch()).find('[data-test="watch-examples"]').exists()).toBe(false)
  })
})

describe('NewsroomWatchEditor, news search source', () => {
  it('asks for a region next to the search words, with the default said in the hint', () => {
    const w = mountEditor(watch())
    expect(w.text()).toContain('Region')
    expect(w.find('input[placeholder="en-US"]').exists()).toBe(true)
    expect(w.text()).toContain('Empty means en-US')
  })

  it('shows a region problem under the region, not under the search words', () => {
    const w = mountEditor(watch('english'))
    expect(w.text()).toContain('A region is a language and a country such as en-GB or en-US.')
    expect(w.find('input[placeholder="en-US"]').attributes('aria-invalid')).toBe('true')
  })
})
