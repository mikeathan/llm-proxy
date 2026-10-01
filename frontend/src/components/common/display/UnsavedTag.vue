<script setup lang="ts">
// The ✱ dashed-emphasis tag for an edited, not yet saved buffer (Phase 5).
withDefaults(defineProps<{ label?: string }>(), { label: "Unsaved changes" })
</script>

<template>
  <span class="inline-flex h-5 items-center border border-dashed border-text-primary px-[7px] font-mono text-[length:var(--text-micro)] font-semibold uppercase tracking-[var(--tracking-micro)] text-primary">{{ label }}</span>
</template>
