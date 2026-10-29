<script setup lang="ts">
import { nextTick, ref, useId, watch } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import { MAX_DEEP_PORTS } from "../../constants/netaudit"

// Asks before a deep check, and says exactly what it will send (Constitution II.3, "Intrusive runs": the operator asks for
// it, for one named device). The server asks for the passphrase again when the button is pressed.
const props = defineProps<{ open: boolean; device: string; ports: readonly number[]; running: boolean; error: string }>()
const emit = defineEmits<{ (e: "run"): void; (e: "cancel"): void }>()

const titleId = useId()
const runButton = ref<HTMLElement | null>(null)
watch(() => props.open, async (open) => {
  if (!open) return
  await nextTick()
  runButton.value?.querySelector("button")?.focus()
}, { immediate: true })
</script>

<template>
  <div v-if="open" class="fixed inset-0 z-50 flex items-center justify-center bg-scrim/60 p-4">
    <div role="dialog" aria-modal="true" :aria-labelledby="titleId" class="flex max-h-full w-full max-w-lg flex-col gap-3 overflow-y-auto rounded-[var(--radius-sm)] border border-strong bg-surface p-5" data-test="deep-dialog" @keydown.escape="!running && emit('cancel')">
      <h2 :id="titleId" class="m-0 text-[15px] font-semibold text-primary">Deep check of {{ device }}?</h2>
      <p class="m-0 text-secondary">This looks harder than a normal scan, so it only runs when you ask. For the {{ Math.min(ports.length, MAX_DEEP_PORTS) }} open {{ ports.length === 1 ? "port" : "ports" }} the last scan found<template v-if="ports.length"> ({{ ports.slice(0, MAX_DEEP_PORTS).join(", ") }})</template>, it will:</p>
      <ul class="m-0 flex list-disc flex-col gap-2 pl-5 text-secondary" data-test="deep-sends">
        <li>On a service that speaks TLS: open about 60 connections in a row to list the TLS versions and ciphers it accepts. A fragile device can slow down or restart under that.</li>
        <li>On an SSH service: start one login with a made-up user name, to see whether it accepts passwords. The device will log a failed login from this computer.</li>
        <li>Guess no passwords and change nothing.</li>
      </ul>
      <p class="m-0 text-[length:var(--text-small)] text-muted">One device only, slowly, and it can take a few minutes. You will be asked for your passphrase. A device can be checked once every ten minutes.</p>
      <p v-if="error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error" data-test="deep-error">{{ error }}</p>
      <div class="flex flex-wrap justify-end gap-2">
        <BaseButton variant="secondary" class-name="max-sm:!h-11" :disabled="running" @click="emit('cancel')">Cancel</BaseButton>
        <span ref="runButton"><BaseButton class-name="max-sm:!h-11" :loading="running" :disabled="ports.length === 0" data-test="deep-run" @click="emit('run')">Run deep check</BaseButton></span>
      </div>
    </div>
  </div>
</template>
