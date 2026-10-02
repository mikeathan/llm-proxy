import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import UserMessage from '../../../../components/AgentIde/assistant/UserMessage.vue'

describe('UserMessage', () => {
  it('offers copying the message as sent, beside sending it again', async () => {
    const w = mount(UserMessage, { props: { content: 'line 1\nline 2' }, global: { stubs: { Icon: true } } })
    const copy = w.findComponent({ name: 'CopyButton' })
    expect(copy.props('text')).toBe('line 1\nline 2')
    expect(copy.props('title')).toBe('Copy message')
    await w.get('button[aria-label^="Send again"]').trigger('click')
    expect(w.emitted('retry')).toEqual([['line 1\nline 2']])
  })
})
