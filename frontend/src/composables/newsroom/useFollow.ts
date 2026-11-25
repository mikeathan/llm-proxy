import { ref } from 'vue'
import { BriefService } from '../../services/newsroom/briefService'
import { NewsroomService } from '../../services/newsroom/newsroomService'
import { BRIEFS_ACTION_FAILED } from '../../constants/newsroom'
import { editionJoining, sectionGroups } from '../../utils/newsroom/briefForm'
import { followTopic, followWatch } from '../../utils/newsroom/storyTuning'
import type { BriefFollowEdit, SourcePreview } from '../../types/newsroom'

const reasonOf = (e: unknown): string => (e instanceof Error && e.message ? e.message : BRIEFS_ACTION_FAILED)

/**
 * Following something from one form (ADR-0086): as a brief topic (it joins every brief that already reads its section), as a watch that alerts, or both.
 * The stored things are the ones the Following tab lists; this is only the one way in. A preview reads what a news search finds for the words now.
 */
export function useFollow() {
  const sections = ref<string[]>([])
  const saving = ref(false)
  const error = ref('')
  const preview = ref<SourcePreview | null>(null)
  let previewing: AbortController | null = null

  async function loadSections() {
    try {
      sections.value = sectionGroups(await BriefService.topics()).filter((g) => !g.own).map((g) => g.label)
    } catch {
      sections.value = []
    }
  }

  async function lookUp(text: string) {
    previewing?.abort()
    previewing = new AbortController()
    try {
      preview.value = await NewsroomService.lookup({ kind: 'newssearch', target: text.trim() }, previewing.signal)
    } catch {
      preview.value = null
    }
  }

  /** Follows it; says how many briefs a new topic joined, or null when it was refused (`error` says why). */
  async function follow(edit: BriefFollowEdit): Promise<{ joined: number } | null> {
    saving.value = true
    error.value = ''
    try {
      let joined = 0
      if (edit.mode !== 'alerts') {
        const topic = await BriefService.createTopic(followTopic(edit))
        const [editions, topics] = await Promise.all([BriefService.editions(), BriefService.topics()])
        for (const e of editions) {
          const join = editionJoining(e, topics, topic)
          if (join) {
            await BriefService.saveEdition(e.id, join)
            joined++
          }
        }
      }
      if (edit.mode !== 'brief') await NewsroomService.create(followWatch(edit))
      return { joined }
    } catch (e) {
      error.value = reasonOf(e)
      return null
    } finally {
      saving.value = false
    }
  }

  return { sections, saving, error, preview, loadSections, lookUp, follow }
}
