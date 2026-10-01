import { onActivated, onDeactivated, onMounted, onUnmounted } from "vue"

/**
 * Runs `task` every `intervalMs` while the owning component is on screen.
 * Pauses while a kept-alive view sits in the background (plan D19) and runs
 * once immediately on return, so the data is never stale when shown. The
 * timer is owned by the component and cleared on unmount.
 */
export function usePolling(task: () => void, intervalMs: number) {
  let timer: ReturnType<typeof setInterval> | null = null

  function start() {
    if (timer === null) timer = setInterval(task, intervalMs)
  }
  function stop() {
    if (timer === null) return
    clearInterval(timer)
    timer = null
  }

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
