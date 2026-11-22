import { computed, onMounted, ref } from 'vue'
import { BriefService } from '../../services/newsroom/briefService'
import { usePolling } from '../ui/usePolling'
import { POLL_INTERVAL_MS } from '../../constants/api'
import { BRIEFS_ACTION_FAILED, BRIEFS_LOAD_FAILED } from '../../constants/newsroom'
import type { BriefEdition, BriefEditionEdit, BriefSettings, BriefIssue, BriefTopic, BriefTopicEdit } from '../../types/newsroom'

const reasonOf = (e: unknown, fallback: string): string => (e instanceof Error && e.message ? e.message : fallback)

/**
 * The Briefs tab's data: the editions, the topics and the editions already made, refreshed on a timer while the page is open. The edition on screen is
 * the one the owner picked, else the first.
 */
export function useBriefs() {
  const editions = ref<BriefEdition[]>([])
  const topics = ref<BriefTopic[]>([])
  const issues = ref<BriefIssue[]>([])
  const settings = ref<BriefSettings | null>(null)
  const pickedId = ref<number | null>(null)
  const loading = ref(true)
  const error = ref<string | null>(null)
  const actionError = ref('')
  // True while an edition is being written by hand: it can take a while on a local model.
  const making = ref(false)
  const saving = ref(false)

  const edition = computed<BriefEdition | null>(() => editions.value.find((e) => e.id === pickedId.value) ?? editions.value[0] ?? null)
  const issue = computed<BriefIssue | null>(() => issues.value.find((i) => i.edition_id === edition.value?.id) ?? null)

  async function load() {
    try {
      const [e, t, i, s] = await Promise.all([BriefService.editions(), BriefService.topics(), BriefService.issues(), BriefService.settings()])
      editions.value = e
      topics.value = t
      issues.value = i
      settings.value = s
      error.value = null
    } catch (err) {
      error.value = reasonOf(err, BRIEFS_LOAD_FAILED)
    } finally {
      loading.value = false
    }
  }

  // Runs one change, reloads what the page shows and keeps the server's reason when it is refused.
  async function change<T>(action: () => Promise<T>): Promise<{ ok: true; value: T } | { ok: false }> {
    saving.value = true
    actionError.value = ''
    try {
      const value = await action()
      await load()
      return { ok: true, value }
    } catch (err) {
      actionError.value = reasonOf(err, BRIEFS_ACTION_FAILED)
      return { ok: false }
    } finally {
      saving.value = false
    }
  }

  async function makeNow(): Promise<boolean> {
    const id = edition.value?.id
    if (id === undefined) return false
    making.value = true
    try {
      return (await change(() => BriefService.makeNow(id))).ok
    } finally {
      making.value = false
    }
  }

  const saveSettings = async (modelCallsPerDay: number): Promise<boolean> => (await change(() => BriefService.saveSettings(modelCallsPerDay))).ok
  const createTopic = async (edit: BriefTopicEdit): Promise<boolean> => (await change(() => BriefService.createTopic(edit))).ok
  const saveTopic = async (id: number, edit: BriefTopicEdit): Promise<boolean> => (await change(() => BriefService.saveTopic(id, edit))).ok
  const removeTopic = async (id: number): Promise<boolean> => (await change(() => BriefService.removeTopic(id))).ok
  const createEdition = async (edit: BriefEditionEdit): Promise<boolean> => (await change(() => BriefService.createEdition(edit))).ok
  const saveEdition = async (id: number, edit: BriefEditionEdit): Promise<boolean> => (await change(() => BriefService.saveEdition(id, edit))).ok
  const removeEdition = async (id: number): Promise<boolean> => (await change(() => BriefService.removeEdition(id))).ok
  async function collectTopic(id: number): Promise<{ new: number; problems?: string[] } | null> {
    const done = await change(() => BriefService.collect(id))
    return done.ok ? done.value : null
  }

  onMounted(load)
  usePolling(load, POLL_INTERVAL_MS)

  return {
    editions, topics, issues, settings, saveSettings, edition, issue, pickedId, loading, error, actionError, making, saving, load, makeNow,
    createTopic, saveTopic, removeTopic, collectTopic, createEdition, saveEdition, removeEdition,
  }
}
