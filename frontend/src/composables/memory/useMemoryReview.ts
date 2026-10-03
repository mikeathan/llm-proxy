import { computed, ref, type Ref } from 'vue'
import { AssistantService } from '../../services/assistant/assistantService'
import { useMemory } from './useMemory'
import type { ReviewItem } from '../../types/memory'

const REVIEW_FAILED = 'The review could not be completed.'
const SAVE_FAILED = 'Some memories could not be saved. They are still listed, so you can try again.'

// "Review this chat for memories": the model proposes, the operator ticks, and only ticked facts are saved — through
// the same Add-memory call as a hand-typed fact. Closing forgets everything; nothing is stored until Save.
export function useMemoryReview(workspaceId: Readonly<Ref<string>>, sessionId: Readonly<Ref<string | null>>) {
  const { createMemory } = useMemory()
  const open = ref(false)
  const loading = ref(false)
  const saving = ref(false)
  const error = ref('')
  const items = ref<ReviewItem[]>([])

  const selectedCount = computed(() => items.value.filter((i) => i.selected).length)

  async function start() {
    const session = sessionId.value
    if (!session) return
    open.value = true
    loading.value = true
    error.value = ''
    items.value = []
    try {
      const suggestions = await AssistantService.reviewMemories(workspaceId.value, session)
      // A fact memory already holds is shown, but never ticked.
      items.value = suggestions.map((s) => ({ ...s, selected: !s.duplicate, failed: false }))
    } catch (err) {
      error.value = err instanceof Error ? err.message : REVIEW_FAILED
    } finally {
      loading.value = false
    }
  }

  function toggle(index: number) {
    const item = items.value[index]
    if (item && !item.duplicate) item.selected = !item.selected
  }

  // Resolves how many facts were saved. The ones that saved leave the list; one that failed stays, marked.
  async function save(): Promise<number> {
    const chosen = items.value.filter((i) => i.selected)
    if (chosen.length === 0) return 0
    saving.value = true
    error.value = ''
    let saved = 0
    try {
      for (const item of chosen) {
        const ok = await createMemory(workspaceId.value, { content: item.content, scope: item.scope, mode: item.mode, keep: 'permanent' })
        item.failed = !ok
        if (ok) saved++
      }
    } finally {
      saving.value = false
    }
    items.value = items.value.filter((i) => !(i.selected && !i.failed))
    if (items.value.some((i) => i.failed)) error.value = SAVE_FAILED
    else close()
    return saved
  }

  function close() {
    open.value = false
    items.value = []
    error.value = ''
  }

  return { open, loading, saving, error, items, selectedCount, start, toggle, save, close }
}
