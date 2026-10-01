<script setup lang="ts">
import Icon from "../../icons/Icon.vue"

// A labelled search field. Debounce belongs to the consumer (e.g. the route
// query it writes), so typing stays responsive.
defineProps<{ modelValue: string; label: string; placeholder?: string }>()
defineEmits<{ (e: "update:modelValue", value: string): void }>()
</script>

<template>
  <span class="relative flex min-w-0 items-center">
    <Icon name="search" size="sm" class-name="pointer-events-none absolute left-2.5 text-faint" />
    <input
      type="search"
      :value="modelValue"
      :aria-label="label"
      :placeholder="placeholder ?? label"
      class="h-8 w-full min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas pl-8 pr-2.5 font-mono text-[length:var(--text-small)] text-primary placeholder:text-faint focus-visible:outline-none focus-visible:ring-2"
      @input="$emit('update:modelValue', ($event.target as HTMLInputElement).value)"
    />
  </span>
</template>
