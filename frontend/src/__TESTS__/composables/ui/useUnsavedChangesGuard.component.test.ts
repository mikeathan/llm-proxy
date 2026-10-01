import { describe, it, expect, afterEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h, ref } from 'vue'
import { createMemoryHistory, createRouter, RouterView } from 'vue-router'
import { useUnsavedChangesGuard } from '../../../composables/ui/useUnsavedChangesGuard'
import { useConfirm } from '../../../composables/ui/useConfirm'

const dirty = ref(false)
// Only file routes replace the buffer; any other navigation keeps it.
const onlyFiles = ref(false)
const Editor = defineComponent({
  setup() {
    useUnsavedChangesGuard(dirty, { discards: (to) => !onlyFiles.value || to.path.startsWith('/files/') })
    return () => h('textarea')
  },
})
const Other = defineComponent({ render: () => h('p', 'other') })
const mounted: { unmount: () => void }[] = []

async function setup() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/files/:name', component: Editor },
      { path: '/other', component: Other },
    ],
  })
  await router.push('/files/a')
  const wrapper = mount(defineComponent({ render: () => h(RouterView) }), { global: { plugins: [router] } })
  await router.isReady()
  mounted.push(wrapper)
  return { router, wrapper }
}

// Answers the pending ConfirmDialog once it opens.
async function answer(confirmed: boolean) {
  await flushPromises()
  const dialog = useConfirm()
  expect(dialog.isOpen.value).toBe(true)
  if (confirmed) dialog.handleConfirm()
  else dialog.handleCancel()
}

describe('useUnsavedChangesGuard', () => {
  afterEach(() => {
    mounted.splice(0).forEach((w) => w.unmount())
    dirty.value = false
    onlyFiles.value = false
  })

  it('lets a clean editor leave without asking', async () => {
    const { router } = await setup()
    await router.push('/other')
    expect(useConfirm().isOpen.value).toBe(false)
    expect(router.currentRoute.value.path).toBe('/other')
  })

  it('keeps a dirty editor when the user stays', async () => {
    const { router } = await setup()
    dirty.value = true
    const navigation = router.push('/other')
    await answer(false)
    await navigation
    expect(router.currentRoute.value.path).toBe('/files/a')
  })

  it('leaves a dirty editor when the user confirms', async () => {
    const { router } = await setup()
    dirty.value = true
    const navigation = router.push('/other')
    await answer(true)
    await navigation
    expect(router.currentRoute.value.path).toBe('/other')
  })

  it('also guards switching to another file on the same route', async () => {
    const { router } = await setup()
    dirty.value = true
    const navigation = router.push('/files/b')
    await answer(false)
    await navigation
    expect(router.currentRoute.value.path).toBe('/files/a')
  })

  it('does not ask when the navigation keeps the buffer', async () => {
    const { router } = await setup()
    dirty.value = true
    onlyFiles.value = true
    await router.push('/other')
    expect(useConfirm().isOpen.value).toBe(false)
    expect(router.currentRoute.value.path).toBe('/other')
  })

  it('warns on tab close only while dirty, and stops listening on unmount', async () => {
    const { wrapper } = await setup()
    const close = () => {
      const event = new Event('beforeunload', { cancelable: true })
      window.dispatchEvent(event)
      return event.defaultPrevented
    }
    expect(close()).toBe(false)
    dirty.value = true
    expect(close()).toBe(true)
    wrapper.unmount()
    expect(close()).toBe(false)
  })
})
