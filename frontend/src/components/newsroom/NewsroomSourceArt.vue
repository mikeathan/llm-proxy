<script setup lang="ts">
import type { SourceKind } from "../../types/newsroom"

// A small line drawing for each kind of source. Decorative: the name is always written beside it. Strokes follow the text colour, so the theme sets them.
defineProps<{ kind: SourceKind }>()
</script>

<template>
  <svg viewBox="0 0 48 48" width="48" height="48" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" class="flex-none text-accent-brand" :data-art="kind">
    <template v-if="kind === 'feed'">
      <circle cx="13" cy="35" r="3" fill="currentColor" stroke="none" />
      <path d="M10 22a16 16 0 0 1 16 16M10 11a27 27 0 0 1 27 27" />
    </template>
    <template v-else-if="kind === 'hackernews'">
      <circle cx="21" cy="21" r="10" />
      <path d="m29 29 10 10M16 18h10M16 24h6" />
    </template>
    <template v-else-if="kind === 'mastodon'">
      <path d="M8 12a4 4 0 0 1 4-4h24a4 4 0 0 1 4 4v18a4 4 0 0 1-4 4H22l-8 7v-7h-2a4 4 0 0 1-4-4Z" />
      <path d="M19 14l-2 14M27 14l-2 14M14 19h16M13 24h16" />
    </template>
    <template v-else-if="kind === 'page'">
      <rect x="6" y="8" width="36" height="32" rx="3" />
      <path d="M6 16h36M12 24h14M12 30h10" />
      <path d="M30 29l4 4 6-8" />
    </template>
    <template v-else>
      <path d="M41 8 6 22l11 4 4 12 5-7 9 7Z" />
      <path d="m17 26 24-18-18 22" />
    </template>
  </svg>
</template>
