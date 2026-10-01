import { describe, it, expect } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '../../../router'
import { useDestinationRoute } from '../../../composables/ui/useDestinationRoute'

describe('useDestinationRoute', () => {
  it('follows routes of its destination and holds the last one elsewhere', async () => {
    const router = createAppRouter(createMemoryHistory())
    await router.push('/workspaces/ws/files/a.md')
    let snapshot!: ReturnType<typeof useDestinationRoute>
    mount(defineComponent({
      setup() {
        snapshot = useDestinationRoute('workspaces')
        return () => h('div')
      },
    }), { global: { plugins: [router] } })

    expect(snapshot.value.params.ws).toBe('ws')
    await router.push('/workspaces/other/memory')
    await flushPromises()
    expect(snapshot.value.params.ws).toBe('other')

    await router.push('/overview')
    await flushPromises()
    expect(snapshot.value.fullPath).toBe('/workspaces/other/memory')
  })
})
