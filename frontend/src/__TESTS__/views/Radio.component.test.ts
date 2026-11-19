import { describe, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '../../router'
import RadioView from '../../views/RadioView.vue'

const STUBS = {
  StackModules: { props: ['stack'], template: '<div data-test="stub-modules" />' },
  RadioListening: { template: '<div data-test="stub-listening" />' },
  RadioScanPlan: { template: '<div data-test="stub-plan" />' },
  RadioKeeping: { template: '<div data-test="stub-keeping" />' },
  StackBoardLink: { template: '<div />' },
}

async function mountAt(path: string) {
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  const wrapper = mount(RadioView, { global: { plugins: [router], stubs: STUBS } })
  await flushPromises()
  return { wrapper, router }
}

describe('RadioView', () => {
  it('has only Listening and Modules: what is to be seen is on the dashboard', async () => {
    const { wrapper } = await mountAt('/radio')
    expect(wrapper.findAll('a.stack-tab').map((a) => a.text())).toEqual(['Listening', 'Modules'])
  })

  it('sends the old Sensors address to its card on the Radio board', async () => {
    const { router } = await mountAt('/radio/sensors')
    expect(router.currentRoute.value.name).toBe('dashboard')
    expect(router.currentRoute.value.query).toEqual({ board: 'radio', card: 'radio-sensors' })
  })
})
