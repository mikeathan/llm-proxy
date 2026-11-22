import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import NewsroomBriefSetup from '../../../components/newsroom/NewsroomBriefSetup.vue'
import type { BriefEdition, BriefTopic } from '../../../types/newsroom'

const topic: BriefTopic = { id: 1, name: 'Technology', queries: ['AI chips'] }
const edition: BriefEdition = { id: 2, name: 'Morning', ready_by: '07:00', days: [1, 2, 3, 4, 5], collect_every: '2h0m0s', collect_for: '12h0m0s', sections: [{ kind: 'news', topic_id: 1 }], enabled: true }

const props = (over: Record<string, unknown> = {}) => ({
  topics: [topic], editions: [edition], settings: { model_calls_per_day: 0, default: 200, max: 2000 }, saving: false, error: '',
  saveTopic: vi.fn().mockResolvedValue(true), saveEdition: vi.fn().mockResolvedValue(true), saveLimit: vi.fn().mockResolvedValue(true), ...over,
})

describe('NewsroomBriefSetup', () => {
  it('lists topics and editions, and says what a topic could not read', () => {
    const w = mount(NewsroomBriefSetup, { props: props({ topics: [{ ...topic, last_problems: ['Feed X: could not be read'] }] }) })
    expect(w.text()).toContain('Technology')
    expect(w.text()).toContain('Ready by 07:00 · 1 topic')
    expect(w.text()).toContain('Feed X: could not be read')
  })

  it('will not offer an edition before there is a topic to put in it', () => {
    const w = mount(NewsroomBriefSetup, { props: props({ topics: [], editions: [] }) })
    expect(w.text()).toContain('Add a topic first')
    expect(w.findAll('button').find((b) => b.text() === 'Add an edition')?.attributes('disabled')).toBeDefined()
  })

  it('saves a new topic from the form and closes it only when the save worked', async () => {
    const p = props()
    const w = mount(NewsroomBriefSetup, { props: p })
    await w.findAll('button').find((b) => b.text() === 'Add a topic')!.trigger('click')
    const inputs = w.findAll('form input[type="text"]')
    await inputs[0]!.setValue('World')
    await w.get('form textarea').setValue('Middle East\nUkraine')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(p.saveTopic).toHaveBeenCalledWith(null, expect.objectContaining({ name: 'World', queries: ['Middle East', 'Ukraine'] }))
    expect(w.find('form textarea').exists()).toBe(false)
  })

  it('keeps the form open when the save is refused', async () => {
    const p = props({ saveTopic: vi.fn().mockResolvedValue(false), error: 'a topic needs a name' })
    const w = mount(NewsroomBriefSetup, { props: p })
    await w.findAll('button').find((b) => b.text() === 'Add a topic')!.trigger('click')
    await w.get('form input[type="text"]').setValue('World')
    await w.get('form textarea').setValue('Ukraine')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(w.find('form textarea').exists()).toBe(true)
    expect(w.get('[role="alert"]').text()).toContain('a topic needs a name')
  })

  it('does not save a topic with nothing to read', async () => {
    const p = props()
    const w = mount(NewsroomBriefSetup, { props: p })
    await w.findAll('button').find((b) => b.text() === 'Add a topic')!.trigger('click')
    await w.get('form input[type="text"]').setValue('Empty')
    await w.get('form').trigger('submit')
    expect(p.saveTopic).not.toHaveBeenCalled()
    expect(w.get('[data-test="topic-needs"]').text()).toContain('at least one search or one feed')
  })

  it('shows the daily limit, saves a number in bounds and refuses one outside them', async () => {
    const p = props({ settings: { model_calls_per_day: 300, default: 200, max: 2000 } })
    const w = mount(NewsroomBriefSetup, { props: p })
    const box = w.get('input[inputmode="numeric"]')
    expect((box.element as HTMLInputElement).value).toBe('300')
    const lastOf = <T>(list: T[]): T => list[list.length - 1]!
    const limitForm = () => lastOf(w.findAll('form'))
    const save = lastOf(w.findAll('button').filter((b) => b.text() === 'Save'))
    expect(save.attributes('disabled')).toBeDefined()
    await box.setValue('5000')
    expect(w.text()).toContain('Use a whole number from 1 to 2000')
    expect(save.attributes('disabled')).toBeDefined()
    await box.setValue('500')
    await limitForm().trigger('submit')
    expect(p.saveLimit).toHaveBeenCalledWith(500)
    await box.setValue('')
    await limitForm().trigger('submit')
    expect(p.saveLimit).toHaveBeenLastCalledWith(0)
  })

  it('saves an edition with the topics ticked, in whole hours', async () => {
    const p = props({ editions: [] })
    const w = mount(NewsroomBriefSetup, { props: p })
    await w.findAll('button').find((b) => b.text() === 'Add an edition')!.trigger('click')
    await w.get('form input[type="text"]').setValue('Evening')
    const topicBox = w.findAll('form [role="group"]')[1]!.get('input[type="checkbox"]')
    await topicBox.setValue(true)
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(p.saveEdition).toHaveBeenCalledWith(null, expect.objectContaining({ name: 'Evening', collect_every: '2h', collect_for: '12h', sections: [{ kind: 'news', topic_id: 1 }] }))
  })
})
