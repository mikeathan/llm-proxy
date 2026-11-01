<script setup lang="ts">
import { ref, useId } from "vue"
import BaseToggle from "../buttons/BaseToggle.vue"
import { MORE_ABOUT_THIS } from "../../../constants/labels"

// A switch with its explanation: the label names the switch, the hint (one short sentence) is announced with it. A longer
// explanation goes in `details`, folded under an expander so the row stays quiet. The row keeps a 44px touch target on phones.
defineProps<{ modelValue: boolean; label: string; hint?: string; details?: string; disabled?: boolean }>()
defineEmits<{ (e: "update:modelValue", value: boolean): void }>()

const id = useId()
const hintId = `${id}-hint`
const detailsId = `${id}-details`
const open = ref(false)
</script>

<template>
  <div class="flex min-w-0 flex-col gap-0.5">
    <div class="flex min-h-11 items-center sm:min-h-8">
      <BaseToggle
        :model-value="modelValue"
        :label="label"
        :disabled="disabled"
        :described-by="hint ? hintId : undefined"
        @update:model-value="$emit('update:modelValue', $event)"
      />
    </div>
    <p v-if="hint" :id="hintId" class="m-0 text-[length:var(--text-small)] text-faint">{{ hint }}</p>
    <template v-if="details">
      <button
        type="button"
        data-test="toggle-details"
        :aria-expanded="open"
        :aria-controls="detailsId"
        class="inline-flex min-h-11 items-center self-start rounded-[var(--radius-sm)] text-[length:var(--text-small)] text-accent-info-text underline-offset-2 hover:underline focus-visible:outline focus-visible:outline-2 focus-visible:outline-focus-ring sm:min-h-8"
        @click="open = !open"
      >{{ MORE_ABOUT_THIS }}</button>
      <p v-show="open" :id="detailsId" data-test="toggle-details-text" class="m-0 text-[length:var(--text-small)] text-muted">{{ details }}</p>
    </template>
  </div>
</template>
