<script setup lang="ts">
import { computed, ref } from 'vue'
import { useMemory } from '../../../composables/memory/useMemory'
import {
  MEMORY_CONTENT_MAX,
  MEMORY_KEEP_LABEL,
  MEMORY_PRIORITY_LABEL,
  MEMORY_RECALL_LABEL,
  MEMORY_SCOPE_LABEL,
  PRIORITY_NORMAL,
} from '../../../constants/memory'
import type { MemoryKeep, MemoryPriority, MemoryRecall, MemoryScope, NewMemory } from '../../../types/memory'
import FormField from '../../common/forms/FormField.vue'
import BaseButton from '../../common/buttons/BaseButton.vue'

// Add a fact by hand: three plain-word choices, no tool names. Saved exactly as
// if the agent had saved it with the same scope / mode / keep.

const props = defineProps<{ workspaceId: string }>()
const emit = defineEmits<{ (e: 'created'): void }>()

const { createMemory, error } = useMemory()

const ALWAYS_HINT = "Always-on facts use part of the model's context window on every run — keep them short."

const content = ref('')
const scope = ref<MemoryScope>('workspace')
const mode = ref<MemoryRecall>('on_demand')
const keep = ref<MemoryKeep>('permanent')
const priority = ref<MemoryPriority>(PRIORITY_NORMAL)
const saving = ref(false)
const saveError = ref('')

const canSave = computed(() => content.value.trim().length > 0 && !saving.value)
const modeHint = computed(() => (mode.value === 'always' ? ALWAYS_HINT : undefined))

async function submit() {
  if (!canSave.value) return
  saving.value = true
  saveError.value = ''
  const fact: NewMemory = { content: content.value.trim(), scope: scope.value, mode: mode.value, keep: keep.value }
  // Priority only means something for always-on facts, and the default is not sent.
  if (mode.value === 'always' && priority.value !== PRIORITY_NORMAL) fact.priority = priority.value
  const saved = await createMemory(props.workspaceId, fact)
  saving.value = false
  if (!saved) {
    saveError.value = error.value ?? 'Could not save this fact.'
    return
  }
  content.value = ''
  priority.value = PRIORITY_NORMAL
  emit('created')
}
</script>

<template>
  <form class="flex flex-col gap-3 rounded-[var(--radius-sm)] border border-hairline bg-surface p-3" novalidate @submit.prevent="submit">
    <FormField label="Fact" :error="saveError">
      <template #default="{ id, describedBy }">
        <textarea
          :id="id"
          v-model="content"
          rows="3"
          :maxlength="MEMORY_CONTENT_MAX"
          :aria-describedby="describedBy"
          class="form-control py-2"
        ></textarea>
      </template>
    </FormField>
    <div class="grid grid-cols-[repeat(auto-fit,minmax(min(100%,200px),1fr))] gap-3">
      <FormField label="Applies to">
        <template #default="{ id, describedBy }">
          <select :id="id" v-model="scope" :aria-describedby="describedBy" class="form-control">
            <option v-for="(label, value) in MEMORY_SCOPE_LABEL" :key="value" :value="value">{{ label }}</option>
          </select>
        </template>
      </FormField>
      <FormField label="Used" :hint="modeHint">
        <template #default="{ id, describedBy }">
          <select :id="id" v-model="mode" :aria-describedby="describedBy" class="form-control">
            <option v-for="(label, value) in MEMORY_RECALL_LABEL" :key="value" :value="value">{{ label }}</option>
          </select>
        </template>
      </FormField>
      <FormField v-if="mode === 'always'" label="Priority" hint="On a small context window, higher-priority facts are cut last.">
        <template #default="{ id, describedBy }">
          <select :id="id" v-model.number="priority" :aria-describedby="describedBy" class="form-control">
            <option v-for="(label, value) in MEMORY_PRIORITY_LABEL" :key="value" :value="Number(value)">{{ label }}</option>
          </select>
        </template>
      </FormField>
      <FormField label="Kept">
        <template #default="{ id, describedBy }">
          <select :id="id" v-model="keep" :aria-describedby="describedBy" class="form-control">
            <option v-for="(label, value) in MEMORY_KEEP_LABEL" :key="value" :value="value">{{ label }}</option>
          </select>
        </template>
      </FormField>
    </div>
    <div class="flex justify-end">
      <BaseButton type="submit" :disabled="!canSave">Remember</BaseButton>
    </div>
  </form>
</template>
