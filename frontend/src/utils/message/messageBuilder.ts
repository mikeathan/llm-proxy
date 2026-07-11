import { ref, type Ref } from 'vue'
import type { AssistantMessage, Segment } from '../../types/assistant'
import type { AgentEvent } from '../../types/dispatcher'
import { getToolCallPayload, getToolResPayload } from '../dispatcher'

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

export function useMessageBuilder(messages: Ref<AssistantMessage[]>) {
  let assistantIdx: number | null = null
  let lastClean = ''
  let reasoningBuffer = ''
  let reasoningCommitted = ''
  let isFinalTurn = false
  let currentToolArgs = ''
  let inReasoningPhase = false

  const streaming = ref(false)
  const thinking = ref(false)
  const liveReasoning = ref('')
  const paused = ref(false)
  let pauseTimer: ReturnType<typeof setTimeout> | null = null

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
      liveReasoning.value = ''
    }
  }

  function render() {
    // Content is set only at turn completion by handleMessage (submit)
    // and finalize (reply push).  During streaming, liveReasoning carries
    // the visible text in the work section; m.content stays empty so
    // finalAnswer stays empty and the thinking‑gap renders.
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
        handleToolStream(ev.payload as string)
        resetPauseTimer()
        return
      case 'reasoning':
        inReasoningPhase = true
        paused.value = false
        streaming.value = true
        thinking.value = true
        handleToolStream(ev.payload as string)
        resetPauseTimer()
        return
      case 'tool_call':
        handleToolCall(getToolCallPayload(ev))
        streaming.value = false
        thinking.value = false
        return
      case 'tool_result':
        handleToolResult(getToolResPayload(ev))
        streaming.value = false
        thinking.value = false
        return
      case 'message':
        handleMessage(ev.payload as AssistantMessage)
        streaming.value = false
        thinking.value = false
        return
    }
  }

  function handleToolStream(text: string) {
    const clean = stripToolCallXml(text)
    if (!clean) return
    if (isFinalTurn) return

    ensureAssistant()

    if (clean.startsWith(lastClean)) {
      reasoningBuffer = clean
      liveReasoning.value = clean
      render()
    } else if (inReasoningPhase) {
      reasoningBuffer = clean
      liveReasoning.value = clean
      render()
      inReasoningPhase = false
    } else {
      commitReasoning()
      reasoningBuffer = clean
      liveReasoning.value = clean
      lastClean = clean
      render()
    }
    lastClean = clean
    forceUpdate()
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

  function handleMessage(payload: AssistantMessage) {
    if (payload.role !== 'assistant') return

    const hasFinal = payload.tool_calls?.some(
      (tc) => tc.function?.name === 'submit_final_answer',
    )

    if (hasFinal) {
      isFinalTurn = true
      commitReasoning()
      const m = assistantMessage()
      if (m) {
        m.content = payload.content || reasoningCommitted
      }
      reasoningBuffer = ''
      lastClean = ''
      return
    }

    commitReasoning()
    lastClean = ''
    render()
  }

  function finalize(reply: string) {
    if (!reply) return
    streaming.value = false
    const m = assistantMessage()
    if (m) {
      m.content = reasoningCommitted
    }
    assistantIdx = null
    messages.value.push({ role: 'assistant', content: reply })
  }

  function reset() {
    if (pauseTimer) { clearTimeout(pauseTimer); pauseTimer = null }
    assistantIdx = null
    lastClean = ''
    reasoningBuffer = ''
    reasoningCommitted = ''
    liveReasoning.value = ''
    isFinalTurn = false
    inReasoningPhase = false
    currentToolArgs = ''
    streaming.value = false
    thinking.value = false
    paused.value = false
  }

  return { handleEvent, finalize, reset, streaming, thinking, liveReasoning, paused, resetPauseTimer }
}
