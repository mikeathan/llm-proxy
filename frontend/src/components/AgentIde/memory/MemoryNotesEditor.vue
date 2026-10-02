<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useMemory } from '../../../composables/memory/useMemory'
import { NOTES_SCOPE_LABEL } from '../../../constants/memory'
import type { NotesScope } from '../../../types/memory'
import type { ChoiceOption } from '../../../types/ui'
import SegmentedControl from '../../common/forms/SegmentedControl.vue'
import FormField from '../../common/forms/FormField.vue'
import BaseButton from '../../common/buttons/BaseButton.vue'

// The operator's own MEMORY.md: standing notes that go first in every run's
// memory, ahead of anything the agent saved. Two files — one for this
// workspace, one for every workspace — kept outside the agent's reach.

const props = defineProps<{ workspaceId: string }>()
const emit = defineEmits<{ (e: 'saved'): void }>()

const { operatorNotes, fetchNotes, saveNotes, error } = useMemory()

const FALLBACK_MAX_CHARS = 6000
const PRECEDENCE_HINT =
  "Goes first in every run's memory, before the facts the agent saved, and is never cut — keep it short on small models. Saved outside the workspace, so the agent cannot change it."

const SCOPES: ChoiceOption[] = (Object.keys(NOTES_SCOPE_LABEL) as NotesScope[]).map((value) => ({ value, label: NOTES_SCOPE_LABEL[value] }))

const scope = ref<NotesScope>('workspace')
const text = ref('')
const saving = ref(false)
const saveError = ref('')

const maxChars = computed(() => operatorNotes.value?.max_chars ?? FALLBACK_MAX_CHARS)
const stored = computed(() => operatorNotes.value?.[scope.value] ?? '')
const tooLong = computed(() => text.value.trim().length > maxChars.value)
const dirty = computed(() => text.value.trim() !== stored.value.trim())
const canSave = computed(() => dirty.value && !tooLong.value && !saving.value)
const formatNumber = (n: number) => n.toLocaleString('en-US')
const counter = computed(() => `${formatNumber(text.value.trim().length)} / ${formatNumber(maxChars.value)}`)

// True once the operator has typed: loaded notes must not overwrite their edits.
const touched = ref(false)

watch(() => props.workspaceId, (ws) => fetchNotes(ws), { immediate: true })
watch(operatorNotes, () => {
  if (!touched.value) text.value = stored.value
}, { immediate: true })
watch(scope, () => {
  text.value = stored.value
  touched.value = false
  saveError.value = ''
})

async function submit() {
  if (!canSave.value) return
  saving.value = true
  saveError.value = ''
  const saved = await saveNotes(props.workspaceId, scope.value, text.value)
  saving.value = false
  if (saved) touched.value = false
  if (!saved) {
    saveError.value = error.value ?? 'Could not save the notes.'
    return
  }
  emit('saved')
}
</script>

<template>
  <form class="flex flex-col gap-3 rounded-[var(--radius-sm)] border border-hairline bg-surface p-3" novalidate @submit.prevent="submit">
    <SegmentedControl v-model="scope" :options="SCOPES" label="Notes apply to" />
    <FormField label="Notes" :hint="PRECEDENCE_HINT" :error="saveError || (tooLong ? 'Too long — shorten the notes to save.' : '')">
      <template #default="{ id, describedBy }">
        <textarea
          :id="id"
          v-model="text"
          @input="touched = true"
          rows="8"
          :aria-describedby="describedBy"
          class="form-control py-2 font-mono"
        ></textarea>
      </template>
    </FormField>
    <div class="flex items-center justify-between gap-2">
      <span class="font-mono text-[length:var(--text-small)]" :class="tooLong ? 'text-state-error' : 'text-muted'">{{ counter }}</span>
      <BaseButton type="submit" :disabled="!canSave">Save notes</BaseButton>
    </div>
  </form>
</template>
