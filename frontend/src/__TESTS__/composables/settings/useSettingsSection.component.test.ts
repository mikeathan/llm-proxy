import { describe, it, expect, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h, ref } from 'vue'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '../../../router'
import { useSettingsSection } from '../../../composables/settings/useSettingsSection'
import { ROUTE_NAMES } from '../../../types/routes'
import type { SettingsTab } from '../../../types/admin'

const KNOWN: SettingsTab[] = ['local', 'mcp']

async function setup(path: string, loaded = true) {
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  const ready = ref(loaded)
  let section!: ReturnType<typeof useSettingsSection>
  mount(defineComponent({
    setup() {
      section = useSettingsSection((tab) => KNOWN.includes(tab), ready)
      return () => h('div')
    },
  }), { global: { plugins: [router] } })
  await flushPromises()
  return { router, ready, section: () => section }
}

describe('useSettingsSection', () => {
  it('reads the section from the route, defaulting to local', async () => {
    expect((await setup('/settings')).section().activeTab.value).toBe('local')
    expect((await setup('/settings/mcp')).section().activeTab.value).toBe('mcp')
  })

  it('shows not-found at the same URL for an unknown section once tabs are known', async () => {
    const { router, ready } = await setup('/settings/bogus', false)
    expect(router.currentRoute.value.name).toBe(ROUTE_NAMES.settings)
    ready.value = true
    // The not-found page is a lazy chunk, so the navigation settles after it loads.
    await vi.waitFor(() => expect(router.currentRoute.value.name).toBe(ROUTE_NAMES.notFound))
    expect(router.currentRoute.value.fullPath).toBe('/settings/bogus')
  })

  it("ignores another destination's section param while leaving", async () => {
    const { router, section } = await setup('/settings/mcp')
    await router.push('/workspaces/ws/memory')
    await flushPromises()
    expect(router.currentRoute.value.name).toBe(ROUTE_NAMES.workspaceSection)
    expect(section().activeTab.value).toBe('mcp')
  })

  it('does not redirect when tabs load after the user has left', async () => {
    const { router, ready } = await setup('/settings/bogus', false)
    await router.push('/overview')
    ready.value = true
    await flushPromises()
    expect(router.currentRoute.value.name).toBe(ROUTE_NAMES.overview)
  })
})
