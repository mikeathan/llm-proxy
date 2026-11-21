import { onScopeDispose, ref } from 'vue'
import { NewsroomService } from '../../services/newsroom/newsroomService'
import { LOOKUP_FAILED } from '../../constants/newsroom'
import type { NewsroomSource, SourceChoice, SourcePreview } from '../../types/newsroom'

// The look-up of one source row in the watch form. Each call cancels the one before it, so a slow answer never replaces a newer one, and nothing is left
// running when the row goes away. A look-up is a hint: its failure is shown as words and never stops the form from saving.
export function useSourceLookup() {
  const preview = ref<SourcePreview | null>(null)
  const loading = ref(false)
  const choices = ref<SourceChoice[]>([])
  const suggestProblem = ref('')
  let lookCtl: AbortController | null = null
  let suggestCtl: AbortController | null = null

  const isAbort = (e: unknown): boolean => e instanceof DOMException && e.name === 'AbortError'

  async function lookUp(source: NewsroomSource): Promise<SourcePreview | null> {
    lookCtl?.abort()
    const ctl = (lookCtl = new AbortController())
    loading.value = true
    try {
      const got = await NewsroomService.lookup(source, ctl.signal)
      if (ctl.signal.aborted) return null
      preview.value = got
      return got
    } catch (e) {
      if (isAbort(e) || ctl.signal.aborted) return null
      preview.value = { found: false, sample: [], choices: [], problem: e instanceof Error && e.message ? e.message : LOOKUP_FAILED }
      return preview.value
    } finally {
      if (lookCtl === ctl) loading.value = false
    }
  }

  async function suggest(source: NewsroomSource): Promise<void> {
    suggestCtl?.abort()
    const ctl = (suggestCtl = new AbortController())
    try {
      const got = await NewsroomService.suggest(source, ctl.signal)
      if (ctl.signal.aborted) return
      choices.value = got.choices ?? []
      suggestProblem.value = got.problem ?? ''
    } catch (e) {
      if (isAbort(e) || ctl.signal.aborted) return
      choices.value = []
      suggestProblem.value = ''
    }
  }

  const clearPreview = (): void => {
    lookCtl?.abort()
    loading.value = false
    preview.value = null
  }
  const clearSuggestions = (): void => {
    suggestCtl?.abort()
    choices.value = []
    suggestProblem.value = ''
  }

  onScopeDispose(() => {
    lookCtl?.abort()
    suggestCtl?.abort()
  })

  return { preview, loading, choices, suggestProblem, lookUp, suggest, clearPreview, clearSuggestions }
}
