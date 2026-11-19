import { inject, onActivated, onDeactivated, onMounted, onUnmounted, watch } from "vue"
import { DASHBOARD_CARD_VISIBLE } from "../../constants/dashboard"

/**
 * Runs `task` every `intervalMs` while the owning component is on screen.
 * Pauses while a kept-alive view sits in the background (plan D19) and runs
 * once immediately on return, so the data is never stale when shown. A tab
 * that comes back to the foreground (a phone with the screen off, a window
 * behind another) also runs it at once: its timers were paused or slowed, so
 * a run that ended meanwhile would otherwise still show as running. The timer
 * and the listener are owned by the component and removed on unmount. Inside a
 * dashboard card the card says whether it is on screen (DASHBOARD_CARD_VISIBLE):
 * a card scrolled out of view skips its ticks and runs once when it returns. A tick
 * while the tab is hidden does nothing (the return above refreshes), so a page
 * left open for days costs the server no requests.
 */
export function usePolling(task: () => void, intervalMs: number) {
  let timer: ReturnType<typeof setInterval> | null = null

  const cardVisible = inject(DASHBOARD_CARD_VISIBLE, null)
  const shown = () => cardVisible === null || cardVisible.value

  function tick() {
    if (document.visibilityState !== "hidden" && shown()) task()
  }
  function onVisible() {
    if (document.visibilityState === "visible") task()
  }
  function start() {
    if (timer !== null) return
    timer = setInterval(tick, intervalMs)
    document.addEventListener("visibilitychange", onVisible)
  }
  function stop() {
    if (timer === null) return
    clearInterval(timer)
    timer = null
    document.removeEventListener("visibilitychange", onVisible)
  }

  if (cardVisible) watch(cardVisible, (now) => { if (now && timer !== null) task() })
  onMounted(start)
  // Also fires right after the first mount inside KeepAlive — the running
  // timer marks that case, so only a genuine return refreshes immediately.
  onActivated(() => {
    if (timer !== null) return
    task()
    start()
  })
  onDeactivated(stop)
  onUnmounted(stop)
}
