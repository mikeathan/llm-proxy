import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'

const read = vi.fn()
const write = vi.fn()
import { useNetauditSwitch } from '../../../composables/netaudit/useNetauditSwitch'

function mountComposable(initial = false) {
  let api!: ReturnType<typeof useNetauditSwitch>
  mount(defineComponent({ setup() { api = useNetauditSwitch(read, write, { load: 'The setting could not be read.', save: 'The setting could not be changed.' }, initial); return () => h('div') } }))
  return api
}

describe('useNetauditSwitch', () => {
  beforeEach(() => {
    read.mockReset()
    write.mockReset()
  })

  it('reads the switch when it mounts', async () => {
    read.mockResolvedValue({ enabled: true })
    const api = mountComposable()
    await flushPromises()
    expect(api.enabled.value).toBe(true)
  })

  it('shows what the server confirmed after a change, and the reason when it could not save', async () => {
    read.mockResolvedValue({ enabled: false })
    const api = mountComposable()
    await flushPromises()
    write.mockResolvedValue({ enabled: true })
    await api.set(true)
    expect(write).toHaveBeenCalledWith(true)
    expect(api.enabled.value).toBe(true)
    write.mockRejectedValue(new Error('disk full'))
    await api.set(false)
    expect(api.enabled.value).toBe(true)
    expect(api.error.value).toContain('disk full')
    expect(api.busy.value).toBe(false)
  })

  it('starts from the given default until the server answers, so an on-by-default switch is not shown off', async () => {
    read.mockReturnValue(new Promise(() => {}))
    expect(mountComposable(true).enabled.value).toBe(true)
  })

  it('says so when the switch cannot be read', async () => {
    read.mockRejectedValue(new Error('down'))
    const api = mountComposable()
    await flushPromises()
    expect(api.error.value).toContain('down')
  })
})
