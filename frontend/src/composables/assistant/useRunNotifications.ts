import { computed, ref, watch, type Ref } from "vue"
import type { Router } from "vue-router"
import type { GlobalRunTick, LaneHolder } from "../../types/assistant"
import type { AutomationRun } from "../../types/dispatcher"
import type { RunEndItem, RunEndOutcome, RunNotification } from "../../types/notifications"
import { ROUTE_NAMES } from "../../types/routes"
import { endedRuns, runIdentity } from "../../utils/runs/runTransitions"
import { toAutomation, toWorkspace, toWorkspaceAssistant } from "../../router/routes"
import { DispatcherService } from "../../services/automation/dispatcherService"
import { useGlobalRunActivity } from "./useGlobalRunActivity"
import { runningActivitySnapshot } from "./useRunningActivity"
import { useToast } from "../useToast"

// Global run notifications (plan Phase 4, D5/D20): terminal transitions only,
// detected from the existing global lane poll — no new timer. The ledger of
// automation runs is read only after an observed end, once per batch.

const DEFAULT_CAP = 20
// Remembered run identities, so a run is announced at most once. Bounded.
const SEEN_LIMIT = 200
const AUTOMATION_KIND = "automation"
const INTERACTIVE_KIND = "interactive"

interface Deps {
  lastTick: Readonly<Ref<GlobalRunTick | null>>
  /** The automation run ledger (read only after an observed end). */
  fetchLedger: () => Promise<AutomationRun[]>
  /** Whether the user is already looking at this run's page. */
  isShowing: (item: RunEndItem) => boolean
  /** The running conversation id known for a workspace, if any. */
  conversationFor: (workspace: string) => string | null
  /** Transient announcement of a new batch (a toast). */
  announce: (message: string) => void
  cap?: number
}

/** The automation run record for an ended run: same automation, recorded after it started. */
function findRunRecord(ledger: AutomationRun[], holder: LaneHolder): AutomationRun | undefined {
  const started = Date.parse(holder.since)
  return ledger
    .filter((run) => run.workspace_id === holder.workspace_id && run.automation_name === holder.automation)
    .filter((run) => Date.parse(run.timestamp) >= started)
    .sort((a, b) => Date.parse(a.timestamp) - Date.parse(b.timestamp))[0]
}

function automationItem(holder: LaneHolder, record: AutomationRun | undefined): RunEndItem {
  const outcome: RunEndOutcome = !record ? "ended" : record.error ? "failed" : "completed"
  return {
    id: runIdentity(holder),
    kind: "automation",
    label: holder.label,
    workspace: holder.workspace_id,
    automationId: holder.key,
    outcome,
    ...(record?.error ? { error: record.error } : {}),
    target: toAutomation(holder.key),
    destination: "automations",
  }
}

export function summarize(items: RunEndItem[]): string {
  if (items.length === 1) {
    const [item] = items as [RunEndItem]
    const what = item.kind === "assistant" ? `Assistant run in ${item.workspace}` : `Automation ${item.label}`
    return `${what} ${item.outcome}`
  }
  const failed = items.filter((item) => item.outcome === "failed").length
  return failed ? `${items.length} runs ended, ${failed} failed` : `${items.length} runs ended`
}

export function createRunNotifications(deps: Deps) {
  const cap = deps.cap ?? DEFAULT_CAP
  const notifications = ref<RunNotification[]>([])
  let baseline: LaneHolder[] | null = null
  let batchSeq = 0
  const seen: string[] = []
  // Automation ends whose run record was missing: re-checked on the next tick, once.
  let awaitingRecord: LaneHolder[] = []
  let queue: Promise<void> = Promise.resolve()

  const unreadCount = computed(() => notifications.value.reduce((n, batch) => n + batch.items.length, 0))

  function remember(id: string): boolean {
    if (seen.includes(id)) return false
    seen.push(id)
    if (seen.length > SEEN_LIMIT) seen.shift()
    return true
  }

  function assistantItem(holder: LaneHolder): RunEndItem {
    const workspace = holder.workspace_id
    const conversation = deps.conversationFor(workspace)
    return {
      id: runIdentity(holder),
      kind: "assistant",
      label: holder.label,
      workspace,
      outcome: "ended",
      target: conversation ? toWorkspaceAssistant(workspace, conversation) : toWorkspace(workspace),
      destination: "workspaces",
    }
  }

  async function resolveAutomations(ended: LaneHolder[]): Promise<RunEndItem[]> {
    const retrying = awaitingRecord
    awaitingRecord = []
    if (!ended.length && !retrying.length) return []
    let ledger: AutomationRun[] = []
    try {
      ledger = await deps.fetchLedger()
    } catch {
      // Treated as "no record yet": the retry (or "ended") covers it.
    }
    const items: RunEndItem[] = []
    for (const holder of ended) {
      const record = findRunRecord(ledger, holder)
      if (record) items.push(automationItem(holder, record))
      else awaitingRecord.push(holder)
    }
    for (const holder of retrying) items.push(automationItem(holder, findRunRecord(ledger, holder)))
    return items
  }

  function publish(items: RunEndItem[]) {
    const visible = items.filter((item) => !deps.isShowing(item))
    if (!visible.length) return
    const batch: RunNotification = { id: `run-batch-${++batchSeq}`, createdAt: Date.now(), items: visible }
    const next = [...notifications.value, batch]
    // Bounded by runs, oldest first out.
    let total = next.reduce((n, b) => n + b.items.length, 0)
    while (total > cap && next.length) {
      const oldest = next[0]!
      const overflow = total - cap
      if (oldest.items.length <= overflow) {
        next.shift()
        total -= oldest.items.length
      } else {
        next[0] = { ...oldest, items: oldest.items.slice(overflow) }
        total -= overflow
      }
    }
    notifications.value = next
    deps.announce(summarize(visible))
  }

  async function processTick(tick: GlobalRunTick) {
    if (tick.status !== "ok") return
    if (baseline === null) {
      baseline = tick.holders
      return
    }
    const ended = endedRuns(baseline, tick.holders).filter((holder) => remember(runIdentity(holder)))
    baseline = tick.holders
    const assistants = ended.filter((holder) => holder.kind === INTERACTIVE_KIND).map(assistantItem)
    const automations = await resolveAutomations(ended.filter((holder) => holder.kind === AUTOMATION_KIND))
    publish([...assistants, ...automations])
  }

  const stop = watch(deps.lastTick, (tick) => {
    if (!tick) return
    // Serialised: a slow ledger read never interleaves with the next tick.
    queue = queue.then(() => processTick(tick))
  })

  function dismiss(batchId: string) {
    notifications.value = notifications.value.filter((batch) => batch.id !== batchId)
  }

  function dismissItem(itemId: string) {
    notifications.value = notifications.value
      .map((batch) => ({ ...batch, items: batch.items.filter((item) => item.id !== itemId) }))
      .filter((batch) => batch.items.length)
  }

  /** Drops the runs whose page the user is now on (called after navigation). */
  function clearShown() {
    notifications.value = notifications.value
      .map((batch) => ({ ...batch, items: batch.items.filter((item) => !deps.isShowing(item)) }))
      .filter((batch) => batch.items.length)
  }

  function clear() {
    notifications.value = []
  }

  return { notifications, unreadCount, dismiss, dismissItem, clearShown, clear, stop, settled: () => queue }
}

type RunNotifications = ReturnType<typeof createRunNotifications>
let instance: RunNotifications | null = null

/** Whether the current route already shows this run (its automation or workspace chat). */
function routeShows(router: Router, item: RunEndItem): boolean {
  const route = router.currentRoute.value
  if (item.kind === "automation") return route.name === ROUTE_NAMES.automation && route.params.id === item.automationId
  return route.name === ROUTE_NAMES.workspaceAssistant && route.params.ws === item.workspace
}

/**
 * Starts the app-wide run notifications once (called from main.ts, the app
 * bootstrap) and clears a notification when its page is opened.
 */
export function startRunNotifications(router: Router): RunNotifications {
  if (instance) return instance
  const { lastTick } = useGlobalRunActivity()
  const { assistantConversationId, polledWorkspaceId } = runningActivitySnapshot()
  const toast = useToast()
  // Last running conversation per workspace, for conversation-level targets.
  const conversations = new Map<string, string>()
  watch([polledWorkspaceId, assistantConversationId], ([ws, conversation]) => {
    if (ws && conversation) conversations.set(ws, conversation)
  })
  const notifications = createRunNotifications({
    lastTick,
    fetchLedger: () => DispatcherService.getGlobalActivity(),
    isShowing: (item) => routeShows(router, item),
    conversationFor: (ws) => conversations.get(ws) ?? null,
    announce: (message) => toast.info(message),
  })
  router.afterEach(() => notifications.clearShown())
  instance = notifications
  return instance
}

/** The app-wide run notifications (started by startRunNotifications). */
export function useRunNotifications(): RunNotifications {
  if (!instance) throw new Error("run notifications are not started")
  return instance
}
