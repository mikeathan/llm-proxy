<script setup lang="ts">
import { nextTick, ref, useId, watch } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import FormField from "../common/forms/FormField.vue"
import SelectInput from "../common/forms/SelectInput.vue"
import { ACCEPT_DAY_OPTIONS, ACCEPT_NOTE_MAX, DEFAULT_ACCEPT_DAYS } from "../../constants/netaudit"

// "This is fine": hide a finding for a while, with an optional note on why. The report asks again when it ends, and
// the server brings the finding back sooner if the service changes.
const props = defineProps<{ open: boolean; what: string; saving: boolean; error: string }>()
const emit = defineEmits<{ (e: "accept", days: number, note: string): void; (e: "cancel"): void }>()

const titleId = useId()
const days = ref(String(DEFAULT_ACCEPT_DAYS))
const note = ref("")
const cancelButton = ref<InstanceType<typeof BaseButton> | null>(null)

watch(() => props.open, async (open) => {
  if (!open) return
  days.value = String(DEFAULT_ACCEPT_DAYS)
  note.value = ""
  await nextTick()
  ;(cancelButton.value?.$el as HTMLElement | undefined)?.focus()
}, { immediate: true })
</script>

<template>
  <div v-if="open" class="fixed inset-0 z-50 flex items-center justify-center bg-scrim/60 p-4">
    <div role="dialog" aria-modal="true" :aria-labelledby="titleId" class="flex w-full max-w-md flex-col gap-3 rounded-[var(--radius-sm)] border border-strong bg-surface p-5" @keydown.escape="emit('cancel')">
      <h2 :id="titleId" class="m-0 text-[15px] font-semibold text-primary">This is fine</h2>
      <p class="m-0 text-secondary">{{ what }} will stop being listed as something to look at. You can take it back at any time.</p>
      <FormField label="For how long">
        <template #default="{ id }"><SelectInput :id="id" v-model="days" :options="ACCEPT_DAY_OPTIONS" label="For how long" /></template>
      </FormField>
      <FormField label="Why (optional)" :hint="`${note.length} of ${ACCEPT_NOTE_MAX}`">
        <template #default="{ id, describedBy }">
          <textarea :id="id" v-model="note" :maxlength="ACCEPT_NOTE_MAX" rows="2" :aria-describedby="describedBy" class="min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas px-2 py-1.5 text-primary focus-visible:outline-none focus-visible:ring-2"></textarea>
        </template>
      </FormField>
      <p v-if="error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ error }}</p>
      <div class="flex flex-wrap justify-end gap-2">
        <BaseButton ref="cancelButton" variant="secondary" @click="emit('cancel')">Cancel</BaseButton>
        <BaseButton :loading="saving" @click="emit('accept', Number(days), note.trim())">This is fine</BaseButton>
      </div>
    </div>
  </div>
</template>
