import { describe, it, expect } from 'vitest'
import source from '../../../../components/AgentIde/assistant/ChatBubble.vue?raw'

// happy-dom has no layout engine, so the phone-width contract is asserted on the
// stylesheet: the step timeline's grid column must be shrinkable, or one long URL
// or tool target widens the timeline past a phone's screen (seen at 390px).
describe('ChatBubble layout', () => {
  it('caps the timeline grid column at the available width', () => {
    const rule = source.match(/\.bubble-inset-wrap\s*\{([^}]*)\}/)?.[1] ?? ''
    expect(rule).toContain('grid-template-columns: minmax(0, 1fr)')
  })
})
