<script setup lang="ts">
import type { ChoiceOption } from "../../../types/ui"

// A labelled native select (native keeps keyboard, mobile pickers and the
// theme's color-scheme for free).
defineProps<{ modelValue: string; options: ChoiceOption[]; label: string }>()
defineEmits<{ (e: "update:modelValue", value: string): void }>()
</script>

<template>
  <select
    :value="modelValue"
    :aria-label="label"
    class="h-8 min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas px-2 font-mono text-[length:var(--text-small)] text-primary focus-visible:outline-none focus-visible:ring-2"
    @change="$emit('update:modelValue', ($event.target as HTMLSelectElement).value)"
  >
    <option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option>
  </select>
</template>
