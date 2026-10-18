import { ref, watch, type Ref } from 'vue'
import { AssistantService } from '../../services/assistant/assistantService'
import type { QueuedMessage, RunEnd } from '../../types/assistant'
import { useToast } from '../useToast'

// A message written while a run is live waits behind that run instead of being refused (sending would replace the run).
// One message, tied to the run it waits behind and the conversation it was written in. It is sent only when that run ended
// as completed and the server confirms no run is live; in every other case it goes back to the box as a draft, so a failed
// or stopped run never fires a message written against an answer that did not arrive, and nothing is lost.

const NOT_SENT = 'Your queued message was not sent'
const DEFAULT_IDLE_WAIT = { attempts: 10, delayMs: 500 }

interface Deps {
  /** The latest run's token and how it ended (useAssistant). `loading` is only used to know whether a run is live. */
  runToken: Readonly<Ref<number>>
  runEnd: Readonly<Ref<RunEnd | null>>
  loading: Readonly<Ref<boolean>>
  conversation: Readonly<Ref<string | null>>
  workspaceId: () => string
  /** Where the waiting message is kept; module-level in the app so it outlives the chat's page. */
  store: Ref<QueuedMessage | null>
  send: (text: string) => Promise<void>
  /** Whether the server has no live chat run in this workspace. */
  isIdle: () => Promise<boolean>
  /** How long to wait for it: the server records a run as over a moment after the run says it completed. */
  idleWait?: { attempts: number; delayMs: number }
  restoreDraft: (text: string) => void
  notify: (message: string) => void
}

export function createMessageQueue(deps: Deps) {
  let queue: Promise<void> = Promise.resolve()

  function take(): QueuedMessage | null {
    const item = deps.store.value
    deps.store.value = null
    return item
  }

  function giveBack(item: QueuedMessage, reason?: string) {
    deps.restoreDraft(item.text)
    if (reason) deps.notify(reason)
  }

  /** Queues the text behind the live run; false when no run is live (the caller sends it) or it is empty. */
  function enqueue(text: string): boolean {
    const trimmed = text.trim()
    if (!trimmed || !deps.loading.value) return false
    deps.store.value = { text: trimmed, workspaceId: deps.workspaceId(), conversationId: deps.conversation.value, runToken: deps.runToken.value }
    return true
  }

  /** Takes the message back out of the queue into the box. */
  function remove() {
    const item = take()
    if (item) deps.restoreDraft(item.text)
  }

  /** On return to the chat: a message whose run ended while it was not on screen goes back to the box, never sent late. */
  function reclaim() {
    if (deps.loading.value) return
    const item = take()
    if (item) giveBack(item, `${NOT_SENT}: the run ended while you were away.`)
  }

  /** Whether the server reports no live run within a few checks. Not confirmed idle means not sent: a send could replace a live run. */
  async function idleSoon(): Promise<boolean> {
    const { attempts, delayMs } = deps.idleWait ?? DEFAULT_IDLE_WAIT
    for (let attempt = 0; attempt < attempts; attempt++) {
      if (attempt > 0) await new Promise((resolve) => setTimeout(resolve, delayMs))
      try {
        if (await deps.isIdle()) return true
      } catch {
        // Treated as busy: the next check may answer.
      }
    }
    return false
  }

  async function settle(item: QueuedMessage, outcome: RunEnd['outcome']) {
    if (outcome !== 'completed') return giveBack(item, `${NOT_SENT} because the run ${outcome === 'failed' ? 'failed' : 'was stopped'}.`)
    if (deps.workspaceId() !== item.workspaceId) return giveBack(item, `${NOT_SENT}: you moved to another workspace.`)
    if (!(await idleSoon())) return giveBack(item, `${NOT_SENT}: a run is still running.`)
    try {
      await deps.send(item.text)
    } catch {
      giveBack(item, `${NOT_SENT}: it could not be sent.`)
    }
  }

  watch(deps.runEnd, (end) => {
    if (!end || deps.store.value?.runToken !== end.token) return
    const item = take()!
    // Serialised: a slow idle check never interleaves with the next ending.
    queue = queue.then(() => settle(item, end.outcome))
  })

  // A first message has no conversation id until its run starts; any other change is the user leaving.
  watch(deps.conversation, (now, before) => {
    if (before === null || now === before) return
    const item = take()
    if (item) giveBack(item, `${NOT_SENT}: you moved to another conversation.`)
  })

  return { queued: deps.store, enqueue, remove, reclaim, settled: () => queue }
}

// Shared by every chat in the app, so a message queued in one page is not lost when the page unmounts.
const queuedMessage = ref<QueuedMessage | null>(null)

interface ChatDeps {
  runToken: Readonly<Ref<number>>
  runEnd: Readonly<Ref<RunEnd | null>>
  loading: Readonly<Ref<boolean>>
  conversation: Readonly<Ref<string | null>>
  workspaceId: () => string
  send: (text: string) => Promise<void>
  restoreDraft: (text: string) => void
}

/** The queue as the chat uses it: the app's shared store, the server's own word on whether a run is live, toasts. */
export function useMessageQueue(deps: ChatDeps) {
  const toast = useToast()
  return createMessageQueue({
    ...deps,
    store: queuedMessage,
    isIdle: async () => !(await AssistantService.getActiveRuns(deps.workspaceId())).assistant_running,
    notify: (message) => toast.info(message),
  })
}
