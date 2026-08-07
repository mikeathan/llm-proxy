import { ref, type Ref } from 'vue'
import type { AssistantMessage, Segment } from '../../types/assistant'
import type { AgentEvent } from '../../types/dispatcher'
import { getToolCallPayload, getToolResPayload, getViolationPayload } from '../dispatcher'

// InsetPhase drives the ChatBubble inset visibility during an agent turn.
//   idle       — no activity yet
//   thinking   — reasoning/tool_stream arriving, no tool call dispatched
//   working    — at least one tool_call dispatched
//   generating — assistant message with content + no tool calls arrived
//   done       — turn finalized (finalize called)
export type InsetPhase = 'idle' | 'thinking' | 'working' | 'generating' | 'done'

export interface MessageBuilderOptions {
  source?: 'chat' | 'automation'
  // automation seeds a synthetic leading user message so groupTurns forms a
  // single clean turn (automation runs have no chat prompt of their own).
  headerMessage?: AssistantMessage
  // 'explicit'  → caller invokes finalize(reply) (chat: HTTP response).
  // 'lifecycle' → builder finalizes on lifecycle{phase:'completed'}
  //               (automation: Hermes-aligned, loop-announced completion).
  finalizeOn?: 'explicit' | 'lifecycle'
}

function stripToolCallXml(text: string): string {
  return text
    .replace(/<tool_call>\s*<\/tool_call>/gi, '')
    .replace(/<tool_result>\s*<\/tool_result>/gi, '')
    .trim()
}

function safeParseArgs(raw: string): string {
  try {
    const parsed = JSON.parse(raw)
    return JSON.stringify(parsed)
  } catch {
    return raw
  }
}

// Adaptive streaming-flush cadence. Each flush re-runs markdown on the ENTIRE
// accumulated live reasoning and swaps its DOM subtree, so the per-flush cost
// grows with the reasoning length (O(n²) over a phase). Short reasoning keeps
// the tight 100ms interval for a responsive stream; as the text grows the
// interval widens toward 250ms, cutting flush frequency ~2.5x exactly where
// the re-parse cost is highest. Pure + O(1), so it stays trivially testable.
const FLUSH_INTERVAL_MIN_MS = 100
const FLUSH_INTERVAL_MAX_MS = 250
const FLUSH_INTERVAL_RAMP_CHARS = 4000

function flushIntervalMs(textLength: number): number {
  const ramp = Math.min(textLength / FLUSH_INTERVAL_RAMP_CHARS, 1)
  return Math.round(
    FLUSH_INTERVAL_MIN_MS + (FLUSH_INTERVAL_MAX_MS - FLUSH_INTERVAL_MIN_MS) * ramp,
  )
}

export function useMessageBuilder(
  messages: Ref<AssistantMessage[]>,
  options: MessageBuilderOptions = {},
) {
  const opts = { source: 'chat' as const, finalizeOn: 'explicit' as const, ...options }
  let finalized = false
  let lastReply = ''

  let assistantIdx: number | null = null
  let lastClean = ''
  let reasoningBuffer = ''
  let reasoningCommitted = ''
  let currentToolArgs = ''
  let inReasoningPhase = false

  const streaming = ref(false)
  const thinking = ref(false)
  const liveReasoning = ref('')
  const paused = ref(false)
  const phase = ref<InsetPhase>('idle')
  let pauseTimer: ReturnType<typeof setTimeout> | null = null

  // Streaming flush throttle. tool_stream events arrive every 10–50ms; each
  // one that touched liveReasoning.value + forceUpdate() triggered a full
  // markdown re-parse and template re-render. We buffer the text and flush at
  // most once per interval, using the adaptive cadence (flushIntervalMs): 100ms
  // for short reasoning, widening to 250ms as the text grows so the O(n²)
  // markdown re-parse + DOM swap of the live block stays bounded. The flush
  // ONLY updates the liveReasoning ref — it deliberately does NOT replace the
  // message object (no forceUpdate). That keeps the turns computed frozen during
  // pure text streaming, so committed turns never re-render or re-parse markdown
  // (the memoized-list pattern ChatGPT/Gemini use). forceUpdate() runs only on
  // segment mutations (tool_call/tool_result/commitReasoning/guardrail), where
  // the turn genuinely changes. All branch logic (reasoningBuffer,
  // commitReasoning, lastClean) stays immediate — only the reactive mirror is
  // deferred.
  let pendingText = ''
  let flushTimer: ReturnType<typeof setTimeout> | null = null

  function scheduleFlush(text: string) {
    pendingText = text
    if (flushTimer) return
    flushTimer = setTimeout(flushLiveReasoning, flushIntervalMs(pendingText.length))
  }

  function flushLiveReasoning() {
    flushTimer = null
    liveReasoning.value = pendingText
  }

  function clearPendingFlush() {
    if (flushTimer) {
      clearTimeout(flushTimer)
      flushTimer = null
    }
    pendingText = ''
  }

  // Transition helper — idempotent: re-setting the same phase is a no-op, and
  // transitions never move backward (done is terminal until reset()).
  function setPhase(next: InsetPhase) {
    if (phase.value === 'done') return
    if (phase.value === next) return
    phase.value = next
  }

  function resetPauseTimer() {
    if (pauseTimer) clearTimeout(pauseTimer)
    pauseTimer = setTimeout(() => { paused.value = true }, 200)
  }

  // Find the index where a new assistant message should be inserted.
  // Normal (fresh session): messages are [system, user], so the slot
  // is right after the last user message = end of array.
  //
  // Loaded history: session loaded mid-run already has completed
  // assistant + tool turns after the user message.  Inserting at
  // the user boundary would place new live events before existing
  // turns, scrambling the order.  Instead, append at the end.
  function findAssistantSlot(): number {
    for (let i = messages.value.length - 1; i >= 0; i--) {
      if (messages.value[i]?.role === 'user') {
        const candidate = i + 1
        if (candidate < messages.value.length) {
          return messages.value.length
        }
        return candidate
      }
    }
    return messages.value.length
  }

  function assistantMessage(): AssistantMessage | undefined {
    if (assistantIdx === null) return undefined
    if (assistantIdx >= messages.value.length) return undefined
    return messages.value[assistantIdx]
  }

  function getSegments(): Segment[] {
    const m = assistantMessage()
    if (!m) return []
    if (!m.segments) m.segments = []
    return m.segments
  }

  function ensureAssistant(): AssistantMessage {
    if (assistantMessage()) return assistantMessage()!
    assistantIdx = findAssistantSlot()
    const newMsg: AssistantMessage = {
      role: 'assistant',
      content: '',
      segments: [],
    }
    messages.value.splice(assistantIdx, 0, newMsg)
    return newMsg
  }

  function commitReasoning() {
    if (reasoningBuffer) {
      reasoningCommitted = reasoningCommitted
        ? reasoningCommitted + '\n\n' + reasoningBuffer
        : reasoningBuffer
      const segments = getSegments()
      segments.push({ kind: 'reasoning', text: reasoningBuffer })
      reasoningBuffer = ''
      clearPendingFlush()
      liveReasoning.value = ''
    }
  }

  function render() {
    // Content is set at turn completion by finalize (reply push).
    // During streaming, liveReasoning carries the visible text; m.content
    // stays empty so finalAnswer stays empty and the thinking‑gap renders.
    assistantMessage()
  }

  function forceUpdate() {
    const idx = assistantIdx
    if (idx !== null && idx < messages.value.length) {
      messages.value[idx] = { ...messages.value[idx] } as AssistantMessage
    }
  }

  function handleEvent(ev: AgentEvent) {
    switch (ev.type) {
      case 'tool_stream':
        inReasoningPhase = false
        paused.value = false
        streaming.value = true
        thinking.value = true
        if (phase.value === 'idle') setPhase('thinking')
        handleToolStream(ev.payload as string)
        resetPauseTimer()
        return
      case 'reasoning':
        inReasoningPhase = true
        paused.value = false
        streaming.value = true
        thinking.value = true
        if (phase.value === 'idle') setPhase('thinking')
        handleToolStream(ev.payload as string)
        resetPauseTimer()
        return
      case 'tool_call':
        if (phase.value === 'thinking' || phase.value === 'idle') setPhase('working')
        handleToolCall(getToolCallPayload(ev))
        streaming.value = false
        thinking.value = false
        return
      case 'tool_result':
        handleToolResult(getToolResPayload(ev))
        streaming.value = false
        thinking.value = false
        return
      case 'guardrail_violation':
        handleGuardrailViolation(getViolationPayload(ev))
        streaming.value = false
        thinking.value = false
        return
      case 'error':
        // Early run failures (e.g. no model configured) arrive on the SSE bus before the agent starts; render them as a visible error segment.
        ensureAssistant()
        getSegments().push({ kind: 'error', message: String((ev.payload as any)?.error ?? 'Unknown error') })
        streaming.value = false
        thinking.value = false
        // Terminal: the run ended before any lifecycle{completed} will fire, so
        // drive the bubble to 'done' and let the owning composable clear loading.
        setPhase('done')
        forceUpdate()
        return
      case 'message':
        // A message with content and no tool calls signals the final answer
        // (Hermes: "no tool calls + substantive content = done"). Finalize from
        // the answer itself so completion never depends on a follow-up
        // lifecycle/HTTP signal that may not arrive for some models.
        const msg = ev.payload as AssistantMessage
        if (msg.role === 'assistant' && msg.content && (!msg.tool_calls || msg.tool_calls.length === 0)) {
          if (!finalized && (phase.value === 'thinking' || phase.value === 'working' || phase.value === 'idle')) {
            setPhase('generating')
          }
          lastReply = msg.content
          finalize(msg.content)
        }
        handleMessage(msg)
        streaming.value = false
        thinking.value = false
        return
      case 'lifecycle': {
        const p = ev.payload as Record<string, any>
        if (p?.phase === 'agent_thinking') {
          // Neutral "working" status at the start of an LLM call, before any
          // response content arrives. Opaque providers (no readable reasoning
          // stream) rely on this to show a spinner/"thinking…" instead of a blank
          // gap. Carries no content, so it can never be mistaken for model output.
          // Idempotent: only flips from idle/done; a later `reasoning` event
          // fills the inset with real text.
          if (phase.value === 'idle' || phase.value === 'done') {
            thinking.value = true
            if (phase.value === 'idle') setPhase('thinking')
          }
          return
        }
        if (opts.finalizeOn === 'lifecycle' && p?.phase === 'completed' && !finalized) {
          finalized = true
          finalize(typeof p.content === 'string' && p.content ? p.content : lastReply)
        }
        return
      }
    }
  }

  function handleToolStream(text: string) {
    const clean = stripToolCallXml(text)
    if (!clean) return

    ensureAssistant()

    if (clean.startsWith(lastClean)) {
      reasoningBuffer = clean
      scheduleFlush(clean)
    } else if (inReasoningPhase) {
      reasoningBuffer = clean
      scheduleFlush(clean)
      inReasoningPhase = false
    } else {
      commitReasoning()
      reasoningBuffer = clean
      lastClean = clean
      scheduleFlush(clean)
    }
    lastClean = clean
  }

  function handleToolCall(tc: { function: { name: string; arguments: string } }) {
    ensureAssistant()
    commitReasoning()

    currentToolArgs = safeParseArgs(tc.function.arguments)
    const segments = getSegments()
    segments.push({
      kind: 'tool_call',
      name: tc.function.name,
      args: currentToolArgs,
      status: 'running',
    })
    forceUpdate()
  }

  function handleToolResult(tr: { name: string, result: any, error?: string }) {
    ensureAssistant()
    const segments = getSegments()
    for (let i = segments.length - 1; i >= 0; i--) {
      const s = segments[i]
      if (s && s.kind === 'tool_call' && s.name === tr.name && s.status === 'running') {
        if (tr.error) {
          s.status = 'error'
          s.error = String(tr.error)
        } else {
          s.status = 'success'
          s.result = typeof tr.result === 'string' ? tr.result : JSON.stringify(tr.result, null, 2)
        }
        forceUpdate()
        break
      }
    }
  }

  // Synchronous guardrail rejections (e.g. path/workspace boundary) are emitted
  // as `guardrail_violation` events with NO preceding tool_call/tool_result
  // events (the backend denies before notifying the tool call). Surface them as
  // their own segment so the user sees the block in the live chat inset.
  function handleGuardrailViolation(payload: { tool: string; error: string }) {
    ensureAssistant()
    const segments = getSegments()
    segments.push({ kind: 'guardrail', tool: payload.tool, error: payload.error })
    forceUpdate()
  }

  function handleMessage(payload: AssistantMessage) {
    if (payload.role !== 'assistant') return

    commitReasoning()
    lastClean = ''
    render()
  }

  function finalize(reply: string) {
    if (finalized) return
    finalized = true
    // The final answer may arrive via the lifecycle `content`, the HTTP reply,
    // the SSE message event (lastReply), or the reasoning stream
    // (liveReasoning / reasoningCommitted). Use whichever carried it so the
    // answer is never lost (Hermes: the substantive final answer must survive)
    // and is rendered exactly once in the result area. Capture the throttled
    // buffer before clearing it — it may hold the final unflushed chunk.
    const pending = pendingText
    clearPendingFlush()
    const answer = reply || lastReply || pending || liveReasoning.value || reasoningCommitted
   
    // Clear transient streaming flags but PRESERVE phase='done' so the final
    // answer renders in the result area. Callers must not reset() right after
    // finalize — that would clobber the done state and hide the answer.
    streaming.value = false
    thinking.value = false
    paused.value = false
    liveReasoning.value = ''
    const m = assistantMessage()
    if (m) {
      m.content = reasoningCommitted
    }
    assistantIdx = null
    messages.value.push({ role: 'assistant', content: answer })
    phase.value = 'done'
  }

  function reset() {
    if (pauseTimer) { clearTimeout(pauseTimer); pauseTimer = null }
    clearPendingFlush()
    assistantIdx = null
    lastClean = ''
    reasoningBuffer = ''
    reasoningCommitted = ''
    liveReasoning.value = ''
    inReasoningPhase = false
    currentToolArgs = ''
    streaming.value = false
    thinking.value = false
    paused.value = false
    phase.value = 'idle'
    finalized = false
    lastReply = ''
  }

  return { handleEvent, finalize, reset, streaming, thinking, liveReasoning, paused, phase, resetPauseTimer }
}
