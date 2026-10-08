<script setup lang="ts">
import { useId } from "vue"
import BaseToggle from "../buttons/BaseToggle.vue"

// A switch with its explanation: the label names the switch, the hint is
// announced with it. The row keeps a 44px touch target on phones.
defineProps<{ modelValue: boolean; label: string; hint?: string; disabled?: boolean }>()
defineEmits<{ (e: "update:modelValue", value: boolean): void }>()

const hintId = `${useId()}-hint`
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
  </div>
</template>
