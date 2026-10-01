import { describe, expect, it, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import Toast from '../../../components/ui/Toast.vue'
import { useToast } from '../../../composables/useToast'

describe('Toast', () => {
  beforeEach(() => {
    useToast().toasts.value = []
  })

  it('announces toasts through a polite live region, errors assertively', async () => {
    const w = mount(Toast, { global: { stubs: { Icon: true } } })
    const region = w.get('[aria-live="polite"]')
    const toast = useToast()
    toast.success('Settings saved')
    toast.error('Could not save: disk full')
    await nextTick()
    expect(region.text()).toContain('Settings saved')
    expect(w.get('[role="alert"]').text()).toContain('disk full')
    expect(w.findAll('[role="alert"]')).toHaveLength(1)
  })

  it('dismisses a toast from its named button', async () => {
    const w = mount(Toast, { global: { stubs: { Icon: true } } })
    useToast().info('Restart requested')
    await nextTick()
    await w.get('button[aria-label="Dismiss notification"]').trigger('click')
    expect(w.text()).not.toContain('Restart requested')
  })
})
