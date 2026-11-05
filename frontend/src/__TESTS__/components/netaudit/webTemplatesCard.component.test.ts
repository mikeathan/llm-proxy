import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { WebTemplatesState } from '../../../types/netaudit'

const service = vi.hoisted(() => ({ webTemplates: vi.fn(), downloadWebTemplates: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

import WebTemplatesCard from '../../../components/netaudit/WebTemplatesCard.vue'

const state = (over: Partial<WebTemplatesState> = {}): WebTemplatesState => ({ present: 0, total: 24, tag: 'v10.5.1', ready: false, reason: 'nuclei is not installed', ...over })

const mountCard = async () => {
  const w = mount(WebTemplatesCard)
  await flushPromises()
  return w
}

describe('WebTemplatesCard', () => {
  beforeEach(() => { service.webTemplates.mockReset().mockResolvedValue(state()); service.downloadWebTemplates.mockReset() })

  it('says how many reviewed checks are held and why the web checks cannot run', async () => {
    const w = await mountCard()
    expect(w.get('[data-test="web-templates-line"]').text()).toBe('0 of 24 reviewed checks are downloaded (release v10.5.1).')
    expect(w.get('[data-test="web-templates-reason"]').text()).toBe('nuclei is not installed')
    expect(w.get('[data-test="web-templates-download"]').text()).toBe('Download checks')
  })

  it('downloads only when the button is pressed and shows the new status', async () => {
    service.downloadWebTemplates.mockResolvedValue(state({ present: 24, ready: true, reason: undefined }))
    const w = await mountCard()
    expect(service.downloadWebTemplates).not.toHaveBeenCalled()
    await w.get('[data-test="web-templates-download"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test="web-templates-line"]').text()).toBe('All 24 reviewed checks are downloaded (release v10.5.1).')
    expect(w.find('[data-test="web-templates-reason"]').exists()).toBe(false)
    expect(w.get('[data-test="web-templates-download"]').text()).toBe('Download checks again')
  })

  it('says plainly when a file was not the reviewed one and keeps the old status', async () => {
    service.downloadWebTemplates.mockRejectedValue(new Error('the web-check templates could not be downloaded: a (it is not the file that was reviewed)'))
    const w = await mountCard()
    await w.get('[data-test="web-templates-download"]').trigger('click')
    await flushPromises()
    expect(w.get('[role="alert"]').text()).toContain('not the file that was reviewed')
    expect(w.get('[data-test="web-templates-line"]').text()).toContain('0 of 24')
  })

  it('offers a retry when the status cannot be read', async () => {
    service.webTemplates.mockRejectedValueOnce(new Error('down')).mockResolvedValue(state())
    const w = await mountCard()
    expect(w.text()).toContain('could not be read')
    await w.get('button').trigger('click')
    await flushPromises()
    expect(w.find('[data-test="web-templates-line"]').exists()).toBe(true)
  })
})
