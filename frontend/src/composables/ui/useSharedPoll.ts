import { onUnmounted } from 'vue'

/**
 * One timer for every component that shows the same server data: it starts with the first consumer, stops with the last, skips its ticks while the page is
 * hidden and runs once when the page comes back. A board with three cards that read one source asks once per interval, not three times. The data lives at
 * module level beside the poller, owned by the composable that created it.
 */
export function sharedPoll(load: () => Promise<void> | void, intervalMs: number) {
  let consumers = 0
  let timer: ReturnType<typeof setInterval> | null = null
  const tick = () => {
    if (document.visibilityState !== 'hidden') void load()
  }
  const onVisible = () => {
    if (document.visibilityState === 'visible') void load()
  }
  function stop() {
    if (timer !== null) clearInterval(timer)
    timer = null
    document.removeEventListener('visibilitychange', onVisible)
  }
  return {
    /** Counts the calling component as a consumer until it unmounts; call it in setup. */
    use() {
      // Taken in setup, not on mount: when a board changes, the new cards are set up before the old ones go, so the count never drops to zero between them.
      if (consumers++ === 0) {
        void load()
        timer = setInterval(tick, intervalMs)
        document.addEventListener('visibilitychange', onVisible)
      }
      onUnmounted(() => {
        if (--consumers === 0) stop()
      })
    },
    /** Asks now, outside the timer (after a change the consumer made). */
    refresh: () => load(),
    /** Test seam: stops the timer and forgets the consumers. */
    reset() {
      stop()
      consumers = 0
    },
  }
}
