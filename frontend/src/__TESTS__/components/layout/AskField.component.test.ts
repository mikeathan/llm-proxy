import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { memoryStorage } from '../../helpers/memoryStorage'
import { ROUTE_NAMES } from '../../../types/routes'
import AskField from '../../../components/layout/AskField.vue'
import { useAskPanel } from '../../../composables/ui/useAskPanel'
import { useLastWorkspace } from '../../../composables/ui/useLastWorkspace'

const empty = { render: () => null }
async function mountField(path = '/overview', mobile = false) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/overview', name: ROUTE_NAMES.overview, component: empty },
      { path: '/workspaces/:ws/assistant/:conversationId?', name: ROUTE_NAMES.workspaceAssistant, component: empty },
    ],
  })
  await router.push(path)
  const w = mount(AskField, { props: { mobile }, global: { plugins: [router], stubs: { Icon: true } } })
  await flushPromises()
  return { w, router }
}

describe('AskField', () => {
  beforeEach(() => {
    vi.stubGlobal('localStorage', memoryStorage())
    useLastWorkspace().remember('demo')
  })
  afterEach(() => {
    useAskPanel().hide()
    useLastWorkspace().forget('demo')
    vi.unstubAllGlobals()
  })

  it('opens and closes the ask panel, and says which it is doing', async () => {
    const { w } = await mountField()
    const field = w.get('button')
    expect(field.attributes('aria-expanded')).toBe('false')
    expect(field.attributes('aria-controls')).toBe('ask-panel')
    await field.trigger('click')
    expect(useAskPanel().open.value).toBe(true)
    expect(w.get('button').attributes('aria-expanded')).toBe('true')
    await w.get('button').trigger('click')
    expect(useAskPanel().open.value).toBe(false)
  })

  it('invites a question in words on a wide screen, and is a named icon on a narrow one', async () => {
    expect((await mountField()).w.text()).toContain('Ask the agent')
    const icon = (await mountField('/overview', true)).w.get('button')
    expect(icon.text()).toBe('')
    expect(icon.attributes('aria-label')).toBe('Ask the assistant')
  })

  it('shrinks to the icon while the panel is open, which has the room', async () => {
    const { w } = await mountField()
    await w.get('button').trigger('click')
    expect(w.get('button').text()).toBe('')
    expect(w.get('button').attributes('aria-label')).toBe('Ask the assistant')
  })

  it('is absent where the page already is the assistant', async () => {
    const { w } = await mountField('/workspaces/demo/assistant')
    expect(w.find('button').exists()).toBe(false)
  })

  it('is absent while there is no workspace to ask', async () => {
    useLastWorkspace().forget('demo')
    const { w } = await mountField()
    expect(w.find('button').exists()).toBe(false)
  })
})
