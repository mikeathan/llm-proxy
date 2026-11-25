import { computed, onMounted, ref, watch } from 'vue'
import { BriefService } from '../../services/newsroom/briefService'
import { usePolling } from '../ui/usePolling'
import { POLL_INTERVAL_MS } from '../../constants/api'
import { BRIEFS_ACTION_FAILED, BRIEFS_LOAD_FAILED, BRIEF_WRITE_POLL_MS } from '../../constants/newsroom'
import type { OpmlFeed, BriefEdition, BriefEditionEdit, BriefIssue, BriefIssueSummary, BriefJob, BriefSettings, BriefSource, BriefTopic, BriefTopicEdit } from '../../types/newsroom'

const reasonOf = (e: unknown, fallback: string): string => (e instanceof Error && e.message ? e.message : fallback)

/**
 * The Newsroom's brief data: the editions and the list of editions written (polled, both light), the one written edition on screen (fetched once by id: its
 * stories never change), and the setup (topics, sources, the daily limit), read when the page opens and after each change. A refresh that a newer one
 * overtook is dropped, so a slow poll never puts back what a save just changed. A write by hand runs on the server; the page follows it until it ends.
 */
export function useBriefs() {
  const editions = ref<BriefEdition[]>([])
  const topics = ref<BriefTopic[]>([])
  const summaries = ref<BriefIssueSummary[]>([])
  const issues = ref<Record<number, BriefIssue>>({})
  const settings = ref<BriefSettings | null>(null)
  const sources = ref<BriefSource[]>([])
  const jobs = ref<BriefJob[]>([])
  const pickedId = ref<number | null>(null)
  const pickedIssueId = ref<number | null>(null)
  const loading = ref(true)
  const error = ref<string | null>(null)
  const actionError = ref('')
  const saving = ref(false)
  let latest = 0

  const edition = computed<BriefEdition | null>(() => editions.value.find((e) => e.id === pickedId.value) ?? editions.value[0] ?? null)
  // The editions written for the picked edition, newest first (the server's order), and the one on screen: the newest unless the owner picked an earlier one.
  const editionIssues = computed<BriefIssueSummary[]>(() => summaries.value.filter((i) => i.edition_id === edition.value?.id))
  const shownSummary = computed(() => editionIssues.value.find((i) => i.id === pickedIssueId.value) ?? editionIssues.value[0] ?? null)
  const issue = computed<BriefIssue | null>(() => (shownSummary.value ? (issues.value[shownSummary.value.id] ?? null) : null))
  const viewingEarlier = computed(() => shownSummary.value !== null && shownSummary.value.id !== editionIssues.value[0]?.id)
  // The write of the edition on screen, while it runs or just after it ended.
  const job = computed(() => jobs.value.find((j) => j.edition_id === edition.value?.id) ?? null)
  const making = computed(() => job.value?.state === 'writing')
  watch(pickedId, () => (pickedIssueId.value = null))

  async function fetchIssue(id: number) {
    if (issues.value[id]) return
    try {
      const got = await BriefService.issue(id)
      issues.value = { ...issues.value, [id]: got }
    } catch (err) {
      error.value = reasonOf(err, BRIEFS_LOAD_FAILED)
    }
  }
  watch(() => shownSummary.value?.id ?? null, (id) => { if (id !== null) void fetchIssue(id) }, { immediate: true })

  async function load() {
    const mine = ++latest
    try {
      const [e, i] = await Promise.all([BriefService.editions(), BriefService.issues()])
      if (mine !== latest) return
      editions.value = e
      summaries.value = i
      error.value = null
    } catch (err) {
      if (mine === latest) error.value = reasonOf(err, BRIEFS_LOAD_FAILED)
    } finally {
      if (mine === latest) loading.value = false
    }
  }

  async function loadSetup() {
    try {
      const [t, s, l] = await Promise.all([BriefService.topics(), BriefService.settings(), BriefService.sources()])
      topics.value = t
      settings.value = s
      sources.value = l
    } catch (err) {
      error.value = reasonOf(err, BRIEFS_LOAD_FAILED)
    }
  }

  // Runs one change, reloads what the page shows and keeps the server's reason when it is refused.
  async function change<T>(action: () => Promise<T>): Promise<{ ok: true; value: T } | { ok: false }> {
    saving.value = true
    actionError.value = ''
    try {
      const value = await action()
      await Promise.all([load(), loadSetup()])
      return { ok: true, value }
    } catch (err) {
      actionError.value = reasonOf(err, BRIEFS_ACTION_FAILED)
      return { ok: false }
    } finally {
      saving.value = false
    }
  }

  // Following a write: the jobs are asked every couple of seconds while one is writing, and not at all otherwise (the tick does nothing).
  async function readJobs() {
    try {
      const before = jobs.value
      jobs.value = await BriefService.jobs()
      for (const j of jobs.value) {
        const was = before.find((b) => b.edition_id === j.edition_id)
        if (was?.state === 'writing' && j.state !== 'writing') await ended(j)
      }
    } catch {
      // The page keeps its last word on the write; the next tick asks again.
    }
  }
  async function ended(j: BriefJob) {
    if (j.state === 'failed') {
      actionError.value = j.problem ?? BRIEFS_ACTION_FAILED
      return
    }
    await load()
    if (j.edition_id === edition.value?.id && j.issue_id) pickedIssueId.value = j.issue_id
  }

  /** Starts writing the edition on screen; the page follows the write and shows it when it ends. */
  async function makeNow(): Promise<boolean> {
    const id = edition.value?.id
    if (id === undefined) return false
    actionError.value = ''
    try {
      const started = await BriefService.makeNow(id)
      jobs.value = [...jobs.value.filter((j) => j.edition_id !== id), started]
      return true
    } catch (err) {
      actionError.value = reasonOf(err, BRIEFS_ACTION_FAILED)
      return false
    }
  }

  const saveSettings = async (modelCallsPerDay: number): Promise<boolean> => (await change(() => BriefService.saveSettings(modelCallsPerDay))).ok
  const createTopic = async (edit: BriefTopicEdit): Promise<boolean> => (await change(() => BriefService.createTopic(edit))).ok
  const saveTopic = async (id: number, edit: BriefTopicEdit): Promise<boolean> => (await change(() => BriefService.saveTopic(id, edit))).ok
  const removeTopic = async (id: number): Promise<boolean> => (await change(() => BriefService.removeTopic(id))).ok
  const createEdition = async (edit: BriefEditionEdit): Promise<boolean> => (await change(() => BriefService.createEdition(edit))).ok
  const saveEdition = async (id: number, edit: BriefEditionEdit): Promise<boolean> => (await change(() => BriefService.saveEdition(id, edit))).ok
  const removeEdition = async (id: number): Promise<boolean> => (await change(() => BriefService.removeEdition(id))).ok
  const addSource = async (label: string, target: string): Promise<boolean> => (await change(() => BriefService.addSource(label, target))).ok
  const removeSource = async (id: number): Promise<boolean> => (await change(() => BriefService.removeSource(id))).ok
  // A list from a feed reader: each feed is added on its own (the server refuses a repeat or a full list), then the page reloads once.
  async function importSources(feeds: OpmlFeed[]): Promise<{ added: number; skipped: number }> {
    let added = 0
    const done = await change(async () => {
      for (const f of feeds) {
        try {
          await BriefService.addSource(f.label, f.target)
          added++
        } catch {
          // refused: counted as skipped below
        }
      }
    })
    return { added: done.ok ? added : 0, skipped: feeds.length - added }
  }
  const deleteAll = async (): Promise<boolean> => {
    const done = await change(() => BriefService.deleteAll())
    if (done.ok) {
      pickedId.value = null
      issues.value = {}
    }
    return done.ok
  }
  async function collectTopic(id: number): Promise<{ new: number; problems?: string[] } | null> {
    const done = await change(() => BriefService.collect(id))
    return done.ok ? done.value : null
  }

  onMounted(() => {
    void load()
    void loadSetup()
    // A write started elsewhere (another tab, the phone) is picked up and followed.
    void readJobs()
  })
  usePolling(load, POLL_INTERVAL_MS)
  usePolling(() => { if (jobs.value.some((j) => j.state === 'writing')) void readJobs() }, BRIEF_WRITE_POLL_MS)

  return {
    editions, topics, settings, sources, addSource, removeSource, importSources, saveSettings, edition, issue, editionIssues, pickedIssueId, viewingEarlier, pickedId, loading, error, actionError, making, job, saving, load, makeNow,
    createTopic, saveTopic, removeTopic, collectTopic, createEdition, saveEdition, removeEdition, deleteAll,
  }
}
