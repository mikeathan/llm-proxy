import { onBeforeUnmount, ref } from 'vue'
import { RadioService } from '../../services/radio/radioService'
import { errorMessage } from '../../utils/errors'
import { RADIO_LISTEN_START_FAILED } from '../../constants/radio'
import type { RadioListenState } from '../../types/radio'

const POLL_MS = 5000
const SESSION_STACK = 'radio'

/** The most uses a session may be approved for (the core's hard limit is 5000). Windows an hour times the hours, plus a margin for a run that restarts. */
function usesFor(state: { channels: number; cycleSeconds: number }, hours: number): number {
  const perHour = Math.ceil((state.channels * 3600) / state.cycleSeconds)
  return Math.min(5000, Math.ceil(perHour * hours) + state.channels * 2)
}

/**
 * Radio · Listening. Starting is three steps and each is undone if a later one fails: ask the stack for the exact list a session must approve,
 * open the bounded session for it (the passphrase is asked for here), then tell the stack to start. Stopping ends the run and every open radio
 * session, so nothing stays authorised after the page says it stopped.
 */
export function useRadioListening() {
  const state = ref<RadioListenState | null>(null)
  const error = ref('')
  const busy = ref(false)
  let timer: ReturnType<typeof setInterval> | null = null

  async function refresh(): Promise<void> {
    try {
      state.value = await RadioService.listening()
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, RADIO_LISTEN_START_FAILED)
    }
    sync()
  }

  /** Polls while a run is going, and not otherwise (no timer when nothing is happening). */
  function sync(): void {
    const want = !!state.value?.listening
    if (want && !timer) timer = setInterval(() => { void refresh() }, POLL_MS)
    if (!want && timer) { clearInterval(timer); timer = null }
  }

  async function start(adapter: string, hours: number): Promise<void> {
    busy.value = true
    error.value = ''
    let sessionId = ''
    try {
      const plan = await RadioService.plan(adapter)
      const session = await RadioService.beginSession({
        stack: SESSION_STACK, label: `Listening on ${adapter}`, hours, requests: plan.requests,
        max_uses: usesFor({ channels: plan.plan.channels.length, cycleSeconds: plan.plan.cycle_seconds }, hours),
      })
      sessionId = session.id
      state.value = await RadioService.start(adapter, hours)
      sync()
    } catch (err) {
      error.value = errorMessage(err, RADIO_LISTEN_START_FAILED)
      if (sessionId) await RadioService.endSession(sessionId).catch(() => {})
    } finally {
      busy.value = false
    }
  }

  async function stop(): Promise<void> {
    busy.value = true
    try {
      state.value = await RadioService.stop()
      const open = await RadioService.openSessions()
      await Promise.all(open.filter((s) => s.stack === SESSION_STACK).map((s) => RadioService.endSession(s.id).catch(() => {})))
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, RADIO_LISTEN_START_FAILED)
    } finally {
      busy.value = false
      sync()
    }
  }

  onBeforeUnmount(() => { if (timer) clearInterval(timer) })

  return { state, error, busy, refresh, start, stop }
}
