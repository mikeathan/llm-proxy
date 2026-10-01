<script setup lang="ts">
// A page's h1, with an optional eyebrow and subtitle, and page actions.
defineProps<{ title: string; eyebrow?: string; subtitle?: string }>()
defineSlots<{ actions?(): unknown }>()
</script>

<template>
  <header class="mb-1 flex flex-wrap items-end justify-between gap-3">
    <div class="min-w-0">
      <p
        v-if="eyebrow"
        class="mb-1.5 font-mono text-[11px] font-medium uppercase tracking-[var(--tracking-micro)] text-muted"
      >
        <span aria-hidden="true" class="mr-2 inline-block h-1.5 w-1.5 bg-accent-brand align-[1px]"></span>{{ eyebrow }}
      </p>
      <h1 class="m-0 text-[length:var(--text-display)] font-[650] leading-[1.1] tracking-[-0.025em] text-primary">{{ title }}</h1>
      <p v-if="subtitle" class="mt-1.5 text-muted">{{ subtitle }}</p>
    </div>
    <div v-if="$slots.actions" class="flex flex-wrap items-center gap-2"><slot name="actions" /></div>
  </header>
</template>
