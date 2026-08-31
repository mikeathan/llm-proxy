import type { AgentEvent } from '../../types'
import { generateId } from '../crypto'

/**
 * O(1) SSE event-id deduper shared by the chat (useAssistantSSE) and
 * automation (useLiveConsole) event consumers. The automation variant used to
 * implement its own dedup with a `liveEvents.some()` array scan (O(n) per
 * event → O(n²) over a long run) and the two implementations drifted apart;
 * a single implementation keeps the dedup semantics identical across channels.
 *
 * Semantics (unchanged from the original consumers):
 *  - an event with an id already seen is rejected;
 *  - a new id is remembered;
 *  - an event without an id always passes and gets an id assigned.
 */
export function createEventIdDeduper() {
  let seen = new Set<string>()

  return {
    /** True when the event is new (should be processed); false when its id was already seen. */
    accept(ev: AgentEvent): boolean {
      if (ev.id && seen.has(ev.id)) return false
      if (ev.id) seen.add(ev.id)
      if (!ev.id) ev.id = generateId()
      return true
    },

    /** Forget every seen id (new run / new session). */
    reset() {
      seen = new Set<string>()
    },
  }
}
