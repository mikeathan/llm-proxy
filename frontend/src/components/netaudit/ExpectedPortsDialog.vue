<script setup lang="ts">
import { computed, nextTick, ref, useId, watch } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import FormField from "../common/forms/FormField.vue"
import { formatPorts, parsePorts } from "../../utils/netaudit/expectations"
import { MAX_EXPECTED_PORTS } from "../../constants/netaudit"

// Edit the ports a device should offer. "Set as expected" on the device saves what is open now; this is for adding a port
// that is closed today, or taking one off. An empty list says nothing should answer.
const props = defineProps<{ open: boolean; device: string; ports: readonly number[]; saving: boolean; error: string }>()
const emit = defineEmits<{ (e: "save", ports: number[]): void; (e: "cancel"): void }>()

const BAD_PORTS = `Use port numbers from 1 to 65535, separated by commas, at most ${MAX_EXPECTED_PORTS}.`
const titleId = useId()
const text = ref("")
const input = ref<HTMLInputElement | null>(null)
const parsed = computed(() => parsePorts(text.value))

watch(() => props.open, async (open) => {
  if (!open) return
  text.value = props.ports.join(", ")
  await nextTick()
  input.value?.focus()
}, { immediate: true })

const save = () => { if (parsed.value) emit("save", parsed.value) }
</script>

<template>
  <div v-if="open" class="fixed inset-0 z-50 flex items-center justify-center bg-scrim/60 p-4">
    <div role="dialog" aria-modal="true" :aria-labelledby="titleId" class="flex w-full max-w-md flex-col gap-3 rounded-[var(--radius-sm)] border border-strong bg-surface p-5" @keydown.escape="emit('cancel')">
      <h2 :id="titleId" class="m-0 text-[15px] font-semibold text-primary">Expected ports for {{ device }}</h2>
      <p class="m-0 text-secondary">Any other open port is reported as a problem, and a listed port that does not answer is reported too. Leave it empty if nothing should answer.</p>
      <FormField label="Ports" :hint="`Now: ${formatPorts(ports)}`">
        <template #default="{ id, describedBy }">
          <input :id="id" ref="input" v-model="text" type="text" inputmode="numeric" :aria-describedby="describedBy" :aria-invalid="parsed === null" class="min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas px-2 py-1.5 font-mono text-primary focus-visible:outline-none focus-visible:ring-2" @keydown.enter.prevent="save" />
        </template>
      </FormField>
      <p v-if="parsed === null" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error" data-test="ports-invalid">{{ BAD_PORTS }}</p>
      <p v-if="error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ error }}</p>
      <div class="flex flex-wrap justify-end gap-2">
        <BaseButton variant="secondary" @click="emit('cancel')">Cancel</BaseButton>
        <BaseButton :loading="saving" :disabled="parsed === null" @click="save">Save</BaseButton>
      </div>
    </div>
  </div>
</template>
