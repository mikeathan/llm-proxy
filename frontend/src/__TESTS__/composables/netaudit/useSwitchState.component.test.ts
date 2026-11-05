import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'

const read = vi.fn()
import { useSwitchState } from '../../../composables/netaudit/useSwitchState'

function mountComposable(initial = false) {
  let api!: ReturnType<typeof useSwitchState>
  mount(defineComponent({ setup() { api = useSwitchState(read, 'The setting could not be read.', initial); return () => h('div') } }))
  return api
}

describe('useSwitchState', () => {
  beforeEach(() => { read.mockReset() })

  it('reads the switch when it mounts', async () => {
    read.mockResolvedValue({ enabled: true })
    const api = mountComposable()
    await flushPromises()
    expect(api.enabled.value).toBe(true)
  })

  it('starts from the given default until the server answers, so an on-by-default switch is not shown off', () => {
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
