<script setup lang="ts">
import { useId } from "vue"

// A labelled form field: the label is bound to the control through the id the
// slot receives; hint and error are announced with it (aria-describedby).
// `tag` sits beside the label (e.g. where a value comes from).
defineProps<{ label: string; hint?: string; error?: string }>()
defineSlots<{ default(props: { id: string; describedBy: string; invalid: boolean }): unknown; tag?(): unknown }>()

const id = useId()
const hintId = `${id}-hint`
</script>

<template>
  <div class="flex min-w-0 flex-col gap-1.5">
    <div v-if="$slots.tag" class="flex flex-wrap items-center justify-between gap-2">
      <label :for="id" class="text-[length:var(--text-small)] font-medium text-secondary">{{ label }}</label>
      <slot name="tag" />
    </div>
    <label v-else :for="id" class="text-[length:var(--text-small)] font-medium text-secondary">{{ label }}</label>
    <slot :id="id" :described-by="hintId" :invalid="!!error" />
    <p v-if="error" :id="hintId" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ error }}</p>
    <p v-else-if="hint" :id="hintId" class="m-0 text-[length:var(--text-small)] text-faint">{{ hint }}</p>
  </div>
</template>
