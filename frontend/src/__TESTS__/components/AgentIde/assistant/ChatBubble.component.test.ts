import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ChatBubble from '../../../../components/AgentIde/assistant/ChatBubble.vue'
import type { Turn } from '../../../../types/message'

const MarkdownViewerStub = {
  props: ['content'],
  template: '<div class="md-stub">{{ content }}</div>',
}

const stubs = {
  MarkdownViewer: MarkdownViewerStub,
  Icon: true,
  ArcOrbitLoader: true,
  ToolCallSegment: true,
}

function turn(overrides: Partial<Turn> = {}): Turn {
  return {
    userMessage: 'list files',
    finalAnswer: '',
    segments: [],
    messages: [],
    ...overrides,
  }
}

function mountBubble(props: Partial<Record<string, unknown>> = {}) {
  return mount(ChatBubble, {
    props: {
      turn: turn(),
      idx: 0,
      loading: true,
      thinking: true,
      liveReasoning: '',
      paused: false,
      isLastTurn: true,
      phase: 'thinking',
      isInsetCollapsed: false,
      isSegExpanded: () => false,
      ...props,
    },
    global: { stubs },
  })
}

describe('ChatBubble live reasoning gating', () => {
  it('renders live reasoning (via MarkdownViewer) in the last (live) turn while streaming', async () => {
    const wrapper = mountBubble({ liveReasoning: 'new run thinking…', isLastTurn: true, phase: 'thinking' })
    expect(wrapper.find('.inset-reasoning--live').exists()).toBe(true)
    // Live reasoning renders with full markdown again (GPU audit confirmed
    // markdown is not a GPU driver; the plain-text experiment was reverted).
    expect(wrapper.find('.inset-reasoning--live .md-stub').text()).toContain('new run thinking…')
  })

  it('does NOT render live reasoning text in a historical (non-last) turn, even while a new run streams', async () => {
    // Regression: after retry, the new run's shared liveReasoning ref used to
    // bleed into the previous (errored/cancelled) turn's expanded inset.
    const wrapper = mountBubble({
      liveReasoning: 'new run thinking…',
      isLastTurn: false,
      idx: 0,
      phase: 'thinking',
    })
    // The inset (and the live block inside it) is kept mounted but hidden via
    // v-show so collapse/expand mid-stream is flicker-free — assert hidden
    // (display:none), not absent.
    const live = wrapper.find('.inset-reasoning--live')
    expect(live.exists()).toBe(true)
    expect(live.attributes('style')).toContain('display: none')
  })

  it('still renders committed error segments in a historical turn', async () => {
    const wrapper = mountBubble({
      turn: turn({ segments: [{ kind: 'error', message: 'connection refused' }] }),
      liveReasoning: 'new run thinking…',
      isLastTurn: false,
      idx: 0,
      phase: 'thinking',
    })
    // The committed error segment stays visible; only the live text is gated.
    expect(wrapper.find('.inset-error').exists()).toBe(true)
    // The live-reasoning block is v-show'd (kept in the DOM to avoid
    // expand/collapse flicker) — it must be hidden via display:none, not shown.
    const live = wrapper.find('.inset-reasoning--live')
    expect(live.exists()).toBe(true)
    expect(live.attributes('style')).toContain('display: none')
    // ...and the new run's live text must not be visible in it.
    expect(wrapper.find('.inset-reasoning--live .md-stub').text()).toContain('new run thinking…')
  })
})
