<script setup lang="ts">
import { APP_TITLE } from "../../config/brand"

// The frame of the sign-in pages: a centred card, a heading, a line of help. They are bare routes (no sidebar), so this is
// the whole page. One column at every width, so it works on a phone.
defineProps<{ title: string; lead?: string }>()
</script>

<template>
  <main class="flex min-h-screen items-center justify-center bg-canvas px-4 py-8 font-sans text-secondary">
    <section class="flex w-full max-w-sm flex-col gap-4 border border-strong bg-surface p-5" :aria-labelledby="'auth-title'">
      <p class="m-0 font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] text-muted">{{ APP_TITLE }}</p>
      <h1 id="auth-title" class="m-0 text-[18px] font-semibold text-primary">{{ title }}</h1>
      <p v-if="lead" class="m-0 text-[length:var(--text-small)] text-secondary">{{ lead }}</p>
      <slot />
    </section>
  </main>
</template>
