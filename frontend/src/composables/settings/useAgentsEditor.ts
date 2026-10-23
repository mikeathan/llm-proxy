import { computed, ref } from 'vue'
import { HttpError } from '../../services/httpClient'
import { ProfileService } from '../../services/profile/profileService'
import { useProfiles } from '../assistant/useProfiles'
import { useConfirm } from '../ui/useConfirm'
import { useToast } from '../useToast'
import { AGENT_DISCARD_PROMPT, agentDeletePrompt, agentSavedToast } from '../../constants/labels'
import { blankInput, copyOf, inputFromDetail, sameInput, slugId, uniqueId } from '../../utils/assistant/agentDraft'
import { errorMessage } from '../../utils/errors'
import type { ProfileDetail, ProfileField, ProfileInput } from '../../types/profile'

const LOAD_AGENT_FAILED = 'The agent could not be read.'
const SAVE_FAILED = 'The agent could not be saved.'
const DELETE_FAILED = 'The agent could not be deleted.'

/**
 * Settings · Agents. One agent is open at a time: an existing one (built-ins read-only) or a new one being written.
 * The server owns validation; its refusal is shown beside the setting it names.
 */
export function useAgentsEditor() {
  const { profiles, error: listError, refresh } = useProfiles()
  const { confirm } = useConfirm()
  const toast = useToast()

  const schema = ref<ProfileField[]>([])
  const schemaError = ref('')
  const detail = ref<ProfileDetail | null>(null)
  const creating = ref(false)
  const draft = ref<ProfileInput>(blankInput())
  const baseline = ref<ProfileInput>(blankInput())
  const newId = ref('')
  const idEdited = ref(false)
  const loading = ref(false)
  const saving = ref(false)
  const loadError = ref('')
  const formError = ref('')
  const fieldErrors = ref<Record<string, string>>({})

  const open = computed(() => creating.value || detail.value !== null)
  const readOnly = computed(() => !creating.value && detail.value?.source === 'builtin')
  const dirty = computed(() => open.value && !readOnly.value && !sameInput(draft.value, baseline.value))
  const currentId = computed(() => (creating.value ? newId.value : (detail.value?.id ?? '')))
  const takenIds = computed(() => profiles.value.map((p) => p.id))

  async function loadSchema() {
    try {
      schema.value = await ProfileService.schema()
      schemaError.value = ''
    } catch (err) {
      schemaError.value = errorMessage(err, 'The agent settings could not be read.')
    }
  }

  function clearErrors() {
    formError.value = ''
    fieldErrors.value = {}
  }

  // Leaving a draft with edits asks first; true means go ahead.
  async function leaveDraft(): Promise<boolean> {
    if (!dirty.value) return true
    return confirm(AGENT_DISCARD_PROMPT)
  }

  function show(next: ProfileDetail) {
    creating.value = false
    detail.value = next
    baseline.value = inputFromDetail(next)
    draft.value = inputFromDetail(next)
    clearErrors()
  }

  async function select(id: string) {
    if (!(await leaveDraft())) return
    loading.value = true
    loadError.value = ''
    try {
      show(await ProfileService.get(id))
    } catch (err) {
      loadError.value = errorMessage(err, LOAD_AGENT_FAILED)
    } finally {
      loading.value = false
    }
  }

  function begin(id: string, input: ProfileInput, blank: ProfileInput) {
    creating.value = true
    detail.value = null
    newId.value = id
    idEdited.value = false
    draft.value = input
    baseline.value = blank
    clearErrors()
  }

  async function startNew() {
    if (!(await leaveDraft())) return
    begin('', blankInput(), blankInput())
  }

  async function duplicate() {
    if (!detail.value || !(await leaveDraft())) return
    const copy = copyOf(detail.value, takenIds.value)
    begin(copy.id, copy.input, blankInput())
    idEdited.value = true
  }

  function setField<K extends keyof ProfileInput>(key: K, value: ProfileInput[K]) {
    if (key === 'name') setName(value as string)
    else draft.value[key] = value
    if (fieldErrors.value[key]) fieldErrors.value = { ...fieldErrors.value, [key]: '' }
  }

  // A new agent's id follows its name until the person types one.
  function setName(name: string) {
    draft.value.name = name
    if (creating.value && !idEdited.value) newId.value = uniqueId(slugId(name), takenIds.value)
  }

  function setId(id: string) {
    newId.value = id
    idEdited.value = true
  }

  async function close() {
    if (!(await leaveDraft())) return
    detail.value = null
    creating.value = false
    clearErrors()
  }

  function discard() {
    draft.value = { ...baseline.value, tools: [...baseline.value.tools], delegates: [...baseline.value.delegates] }
    clearErrors()
  }

  async function save() {
    const id = currentId.value
    saving.value = true
    clearErrors()
    try {
      const saved = await ProfileService.save(id, draft.value)
      show(saved)
      toast.success(agentSavedToast(saved.name))
      await refresh()
    } catch (err) {
      showSaveError(err, SAVE_FAILED)
    } finally {
      saving.value = false
    }
  }

  function showSaveError(err: unknown, fallback: string) {
    const message = errorMessage(err, fallback)
    if (err instanceof HttpError && err.field) fieldErrors.value = { [err.field]: message }
    else formError.value = message
  }

  async function remove() {
    const target = detail.value
    if (!target || target.source === 'builtin') return
    if (!(await confirm(agentDeletePrompt(target, profiles.value)))) return
    saving.value = true
    clearErrors()
    try {
      const result = await ProfileService.remove(target.id)
      await refresh()
      if (result.restored_builtin) show(await ProfileService.get(target.id))
      else {
        detail.value = null
        draft.value = blankInput()
      }
    } catch (err) {
      showSaveError(err, DELETE_FAILED)
    } finally {
      saving.value = false
    }
  }

  void loadSchema()

  return {
    profiles, listError, reload: refresh, schema, schemaError, detail, creating, draft, newId, loading, saving, loadError, formError, fieldErrors,
    open, readOnly, dirty, currentId,
    select, startNew, duplicate, setField, setId, close, discard, save, remove, loadSchema,
  }
}
