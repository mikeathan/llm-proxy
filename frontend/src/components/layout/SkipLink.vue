<script setup lang="ts">
// The first tab stop of the shell (Phase 6): lets a keyboard user skip the
// sidebar and header. Focus is moved in script rather than through the URL
// hash, so the router never sees a navigation.
const props = defineProps<{ target: string }>()

function skip(event: Event) {
  const el = document.getElementById(props.target)
  if (!el) return
  event.preventDefault()
  el.focus()
}
</script>

<template>
  <a
    :href="`#${target}`"
    class="sr-only focus:not-sr-only focus:fixed focus:left-3 focus:top-3 focus:z-[10000] focus:rounded-[var(--radius-sm)] focus:border focus:border-strong focus:bg-surface focus:px-3 focus:py-2 focus:font-mono focus:text-[length:var(--text-small)] focus:text-primary focus:outline-none focus:ring-2"
    @click="skip"
  >Skip to content</a>
</template>
