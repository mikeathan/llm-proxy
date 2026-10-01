import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import EmptyState from '../../../../components/common/feedback/EmptyState.vue'
import ErrorState from '../../../../components/common/feedback/ErrorState.vue'
import LoadingState from '../../../../components/common/feedback/LoadingState.vue'
import Callout from '../../../../components/common/feedback/Callout.vue'

describe('feedback states', () => {
  it('EmptyState states what is missing and offers the next action', () => {
    const w = mount(EmptyState, { props: { title: 'No automations yet', body: 'Create one to run a task on a schedule.' }, slots: { action: '<a href="#">Create</a>' } })
    expect(w.get('h3').text()).toBe('No automations yet')
    expect(w.text()).toContain('Create one')
    expect(w.find('a').exists()).toBe(true)
  })

  it('ErrorState is an alert giving the cause and the next step', () => {
    const w = mount(ErrorState, { props: { title: 'Could not load runs', cause: 'The server did not respond.', next: 'Check that llm-proxy is running, then retry.' } })
    expect(w.attributes('role')).toBe('alert')
    expect(w.text()).toContain('Error')
    expect(w.text()).toContain('The server did not respond.')
    expect(w.text()).toContain('then retry')
  })

  it('LoadingState announces what is loading and draws shape placeholders', () => {
    const w = mount(LoadingState, { props: { label: 'Loading runs', rows: 3 } })
    expect(w.attributes('role')).toBe('status')
    expect(w.text()).toContain('Loading runs')
    expect(w.findAll('[data-test="skeleton"]')).toHaveLength(3)
  })

  it('Callout names its tone and shows a title, body and actions', () => {
    const w = mount(Callout, {
      props: { tone: 'warning', title: 'Agent network is unrestricted' },
      slots: { default: 'Restricting is recommended.', actions: '<button>Restrict</button>' },
    })
    expect(w.attributes('data-tone')).toBe('warning')
    expect(w.text()).toContain('Agent network is unrestricted')
    expect(w.text()).toContain('Restricting is recommended.')
    expect(w.find('button').text()).toBe('Restrict')
  })
})
