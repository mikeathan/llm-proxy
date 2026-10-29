<script setup lang="ts">
import { nextTick, ref, useId, watch } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import PassphraseInput from "./PassphraseInput.vue"
import { useReauth } from "../../composables/auth/useReauth"

// "Confirm it's you": shown when an action (wiping data, changing a secret, turning the sandbox off) needs the passphrase
// again. Confirming sends the action once more; cancelling leaves it undone.
const { open, saving, error, submit, cancel } = useReauth()
const titleId = useId()
const passphrase = ref("")
const form = ref<HTMLElement | null>(null)

watch(open, async (isOpen) => {
  if (!isOpen) return
  passphrase.value = ""
  await nextTick()
  form.value?.querySelector("input")?.focus()
})
</script>

<template>
  <div v-if="open" class="fixed inset-0 z-50 flex items-center justify-center bg-scrim/60 p-4">
    <div role="dialog" aria-modal="true" :aria-labelledby="titleId" class="flex w-full max-w-sm flex-col gap-3 rounded-[var(--radius-sm)] border border-strong bg-surface p-5" @keydown.escape="cancel">
      <h2 :id="titleId" class="m-0 text-[15px] font-semibold text-primary">Confirm it's you</h2>
      <p class="m-0 text-secondary">This action needs your passphrase again.</p>
      <form ref="form" class="flex flex-col gap-3" novalidate @submit.prevent="submit(passphrase)">
        <PassphraseInput v-model="passphrase" label="Passphrase" autocomplete="current-password" :error="error" />
        <div class="flex flex-wrap justify-end gap-2">
          <BaseButton variant="secondary" class-name="max-sm:!h-11" @click="cancel">Cancel</BaseButton>
          <BaseButton type="submit" :loading="saving" :disabled="!passphrase" class-name="max-sm:!h-11">Confirm</BaseButton>
        </div>
      </form>
    </div>
  </div>
</template>
