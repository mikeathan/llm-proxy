import { ref, onUnmounted } from 'vue'

export function useElapsedTimer() {
  const seconds = ref(0)
  let interval: ReturnType<typeof setInterval> | null = null

  function start() {
    seconds.value = 0
    if (interval) return
    interval = setInterval(() => { seconds.value++ }, 1000)
  }

  function stop() {
    if (interval) {
      clearInterval(interval)
      interval = null
    }
  }

  onUnmounted(stop)

  return { seconds, start, stop }
}
