import { onMounted, ref, watch } from 'vue'
import { NewsroomService } from '../../services/newsroom/newsroomService'
import { usePolling } from '../ui/usePolling'
import { POLL_INTERVAL_MS } from '../../constants/api'
import { NEWSROOM_ACTION_FAILED, NEWSROOM_LOAD_FAILED } from '../../constants/newsroom'
import { browserTimeZone } from '../../utils/format/timezone'
import type { NewsroomDay, NewsroomFeedFilter, NewsroomRunOutcome, NewsroomWatch, NewsroomWatchEdit } from '../../types/newsroom'

const reasonOf = (e: unknown, fallback: string): string => (e instanceof Error && e.message ? e.message : fallback)

/**
 * The Newsroom page's data: the watches and the feed under the current filter, refreshed on a timer while the page is on screen and `active` says the
 * tab that shows them is open (the brief, the page's first tab, does not need them). A refresh that was overtaken by a newer one (a filter changed while it
 * was in flight) is dropped, so the feed never shows the answer to an old filter.
 */
export function useNewsroom(active: () => boolean = () => true) {
  const watches = ref<NewsroomWatch[]>([])
  const feed = ref<NewsroomDay[]>([])
  const filter = ref<NewsroomFeedFilter>({ watch: null, unreadOnly: false, days: null })
  const loading = ref(true)
  const error = ref<string | null>(null)
  // The server's reason for the last refused or failed change; empty when none.
  const actionError = ref('')
  const saving = ref(false)
  // The watch whose check is under way, so its row can say so; null when none is running.
  const runningId = ref<number | null>(null)
  let latest = 0

  async function load() {
    if (!active()) return
    const mine = ++latest
    try {
      const [t, f] = await Promise.all([NewsroomService.watches(), NewsroomService.feed(filter.value, browserTimeZone())])
      if (mine !== latest) return
      watches.value = t
      feed.value = f
      error.value = null
    } catch (e) {
      if (mine === latest) error.value = reasonOf(e, NEWSROOM_LOAD_FAILED)
    } finally {
      if (mine === latest) loading.value = false
    }
  }

  async function change<T>(action: () => Promise<T>): Promise<{ ok: true; value: T } | { ok: false }> {
    saving.value = true
    actionError.value = ''
    try {
      const value = await action()
      await load()
      return { ok: true, value }
    } catch (e) {
      actionError.value = reasonOf(e, NEWSROOM_ACTION_FAILED)
      return { ok: false }
    } finally {
      saving.value = false
    }
  }

  const create = async (edit: NewsroomWatchEdit): Promise<boolean> => (await change(() => NewsroomService.create(edit))).ok
  const save = async (id: number, edit: NewsroomWatchEdit): Promise<boolean> => (await change(() => NewsroomService.save(id, edit))).ok
  const remove = async (id: number): Promise<boolean> => (await change(() => NewsroomService.remove(id))).ok
  const markRead = async (ids: number[], read: boolean): Promise<boolean> => (await change(() => NewsroomService.markRead(ids, read))).ok
  const clear = async (what: { ids: number[] } | { scope: 'read' | 'all' }): Promise<number | null> => {
    const done = await change(() => NewsroomService.clear(what))
    return done.ok ? done.value : null
  }
  async function run(id: number): Promise<NewsroomRunOutcome | null> {
    runningId.value = id
    try {
      const done = await change(() => NewsroomService.run(id))
      return done.ok ? done.value : null
    } finally {
      runningId.value = null
    }
  }

  watch(filter, () => void load(), { deep: true })
  watch(active, (on) => { if (on) void load() })
  onMounted(load)
  usePolling(load, POLL_INTERVAL_MS)

  return { watches, feed, filter, loading, error, actionError, saving, runningId, load, create, save, remove, markRead, clear, run }
}
