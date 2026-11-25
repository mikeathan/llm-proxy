<script setup lang="ts">
// A page's h1, with an optional eyebrow and subtitle, and page actions. On a phone the title and the actions share one row, so the eyebrow and
// the subtitle are left out below sm and the title is smaller. `phoneContext` keeps them for a page whose eyebrow and subtitle carry the
// page's own facts (one automation: its workspace and its trigger). The actions wrap under the title when they do not fit beside it.
defineProps<{ title: string; eyebrow?: string; subtitle?: string; phoneContext?: boolean }>()
defineSlots<{ actions?(): unknown }>()
</script>

<template>
  <header class="mb-1 flex flex-wrap items-end justify-between gap-3 max-sm:items-center">
    <div class="min-w-0">
      <p
        v-if="eyebrow"
        :class="['mb-1.5 font-mono text-[11px] font-medium uppercase tracking-[var(--tracking-micro)] text-muted', phoneContext ? '' : 'max-sm:hidden']"
        data-test="page-eyebrow"
      >
        <span aria-hidden="true" class="mr-2 inline-block h-1.5 w-1.5 bg-accent-brand align-[1px]"></span>{{ eyebrow }}
      </p>
      <h1 class="m-0 text-[length:var(--text-title)] font-[650] leading-[1.1] tracking-[-0.025em] text-primary sm:text-[length:var(--text-display)]">{{ title }}</h1>
      <p v-if="subtitle" :class="['mt-1.5 text-muted', phoneContext ? '' : 'max-sm:hidden']" data-test="page-subtitle">{{ subtitle }}</p>
    </div>
    <div v-if="$slots.actions" class="flex flex-wrap items-center gap-2"><slot name="actions" /></div>
  </header>
</template>
