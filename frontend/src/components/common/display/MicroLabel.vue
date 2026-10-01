<script setup lang="ts">
// Mono, uppercase, tracked, muted: the label above a value or a group.
defineSlots<{ default(): unknown }>()
</script>

<template>
  <span class="font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] text-muted"><slot /></span>
</template>
