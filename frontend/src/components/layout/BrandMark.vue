<script setup lang="ts">
// The split-disc brand mark (signed off in Phase 0): two half-discs, primary
// ink (client side) and brand accent (model side), parted by a thin gap — the
// proxy is the seam. Decorative; the product name always sits beside it.
// `muted` draws it in decorative ink, as ornament in empty / not-found states.
withDefaults(defineProps<{ size?: number; muted?: boolean }>(), { size: 16, muted: false })
</script>

<template>
  <svg class="block flex-none" viewBox="0 0 16 16" :width="size" :height="size" aria-hidden="true" focusable="false">
    <path :class="muted ? 'fill-text-decorative' : 'fill-text-primary'" d="M7.4 1a7 7 0 0 0 0 14z" />
    <path :class="muted ? 'fill-text-decorative/50' : 'fill-accent-brand'" d="M8.6 1a7 7 0 0 1 0 14z" />
  </svg>
</template>
