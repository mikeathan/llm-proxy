import { describe, it, expect, vi } from 'vitest'
import { nextTick, ref, shallowRef } from 'vue'
import { createMessageQueue } from '../../../composables/assistant/useMessageQueue'
import type { QueuedMessage, RunEnd } from '../../../types/assistant'

function setup(opts: { idle?: boolean | boolean[]; sendFails?: boolean } = {}) {
  const runToken = ref(1)
  const runEnd = shallowRef<RunEnd | null>(null)
  const loading = ref(true)
  const conversation = ref<string | null>('c1')
  const workspace = ref('ws')
  const store = ref<QueuedMessage | null>(null)
  const send = vi.fn(async (_text: string) => {
    if (opts.sendFails) throw new Error('send failed')
  })
  const answers = Array.isArray(opts.idle) ? [...opts.idle] : null
  const isIdle = vi.fn(async (): Promise<boolean> => (answers ? (answers.shift() ?? false) : opts.idle !== false))
  const restoreDraft = vi.fn()
  const notify = vi.fn()
  const queue = createMessageQueue({
    runToken, runEnd, loading, conversation, workspaceId: () => workspace.value, store, send, isIdle, restoreDraft, notify, idleWait: { attempts: 3, delayMs: 0 },
  })
  const end = async (outcome: RunEnd['outcome'], token = runToken.value) => {
    runEnd.value = { token, outcome }
    await nextTick()
    await queue.settled()
  }
  return { queue, store, runToken, loading, conversation, workspace, send, isIdle, restoreDraft, notify, end }
}

describe('message queue', () => {
  it('keeps what was written behind the live run, tied to that run and conversation', () => {
    const { queue, store } = setup()
    expect(queue.enqueue('  next question  ')).toBe(true)
    expect(store.value).toEqual({ text: 'next question', workspaceId: 'ws', conversationId: 'c1', runToken: 1 })
  })

  it('refuses when no run is live (the caller sends it) and ignores an empty message', () => {
    const { queue, loading, store } = setup()
    expect(queue.enqueue('   ')).toBe(false)
    loading.value = false
    expect(queue.enqueue('hello')).toBe(false)
    expect(store.value).toBeNull()
  })

  it('sends it once when that run completed and the server has no run live', async () => {
    const { queue, send, restoreDraft, store, end } = setup()
    queue.enqueue('next')
    await end('completed')
    expect(send).toHaveBeenCalledExactlyOnceWith('next')
    expect(restoreDraft).not.toHaveBeenCalled()
    expect(store.value).toBeNull()
  })

  it('does not send while the server still reports a live run, because that would replace it', async () => {
    const { queue, send, restoreDraft, notify, end } = setup({ idle: false })
    queue.enqueue('next')
    await end('completed')
    expect(send).not.toHaveBeenCalled()
    expect(restoreDraft).toHaveBeenCalledWith('next')
    expect(notify).toHaveBeenCalledWith(expect.stringContaining('still running'))
  })

  // The server records the run as over a moment after the run says it completed.
  it('waits a moment for the server to release the finished run before it sends', async () => {
    const { queue, send, isIdle, restoreDraft, end } = setup({ idle: [false, false, true] })
    queue.enqueue('next')
    await end('completed')
    expect(isIdle).toHaveBeenCalledTimes(3)
    expect(send).toHaveBeenCalledExactlyOnceWith('next')
    expect(restoreDraft).not.toHaveBeenCalled()
  })

  it('gives up after a few checks and returns the message instead of sending over a live run', async () => {
    const { queue, send, isIdle, restoreDraft, end } = setup({ idle: false })
    queue.enqueue('next')
    await end('completed')
    expect(isIdle).toHaveBeenCalledTimes(3)
    expect(send).not.toHaveBeenCalled()
    expect(restoreDraft).toHaveBeenCalledWith('next')
  })

  it.each([
    ['failed', 'failed'],
    ['stopped', 'stopped'],
  ] as const)('puts the message back in the box when the run %s, and says why', async (outcome, word) => {
    const { queue, send, restoreDraft, notify, end } = setup()
    queue.enqueue('next')
    await end(outcome)
    expect(send).not.toHaveBeenCalled()
    expect(restoreDraft).toHaveBeenCalledWith('next')
    expect(notify).toHaveBeenCalledWith(expect.stringContaining(word))
  })

  it('ignores the end of a different run', async () => {
    const { queue, send, restoreDraft, store, end } = setup()
    queue.enqueue('next')
    await end('completed', 0)
    expect(send).not.toHaveBeenCalled()
    expect(restoreDraft).not.toHaveBeenCalled()
    expect(store.value).not.toBeNull()
  })

  it('puts it back when the user moves to another conversation or workspace, but not when a first run gets its id', async () => {
    const a = setup()
    a.conversation.value = null
    a.queue.enqueue('next')
    a.conversation.value = 'c1'
    await nextTick()
    expect(a.restoreDraft).not.toHaveBeenCalled()
    a.conversation.value = 'c2'
    await nextTick()
    expect(a.restoreDraft).toHaveBeenCalledWith('next')

    const b = setup()
    b.queue.enqueue('next')
    b.workspace.value = 'other'
    await b.end('completed')
    expect(b.send).not.toHaveBeenCalled()
    expect(b.restoreDraft).toHaveBeenCalledWith('next')
  })

  it('gives the text back when it is removed, and says nothing', () => {
    const { queue, store, restoreDraft, notify } = setup()
    queue.enqueue('next')
    queue.remove()
    expect(restoreDraft).toHaveBeenCalledWith('next')
    expect(store.value).toBeNull()
    expect(notify).not.toHaveBeenCalled()
  })

  it('holds one message: a second replaces the first', () => {
    const { queue, store } = setup()
    queue.enqueue('first')
    queue.enqueue('second')
    expect(store.value?.text).toBe('second')
  })

  it('puts the message back if sending it fails, so nothing is lost', async () => {
    const { queue, restoreDraft, notify, end } = setup({ sendFails: true })
    queue.enqueue('next')
    await end('completed')
    expect(restoreDraft).toHaveBeenCalledWith('next')
    expect(notify).toHaveBeenCalled()
  })

  it('on return to the chat, a message whose run is no longer live goes back to the box instead of being sent', () => {
    const { queue, store, loading, restoreDraft } = setup()
    queue.enqueue('next')
    loading.value = false // the run ended while the chat was not on screen
    queue.reclaim()
    expect(restoreDraft).toHaveBeenCalledWith('next')
    expect(store.value).toBeNull()
  })

  it('on return to the chat, a message behind a run that is still live stays queued', () => {
    const { queue, store, restoreDraft } = setup()
    queue.enqueue('next')
    queue.reclaim()
    expect(restoreDraft).not.toHaveBeenCalled()
    expect(store.value).not.toBeNull()
  })
})
