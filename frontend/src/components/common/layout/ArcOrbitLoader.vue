<script setup lang="ts">
withDefaults(defineProps<{
  active: boolean
  radius?: string
  thickness?: number
  color?: string
}>(), {
  radius: '1rem',
  thickness: 1.5,
  color: '129, 140, 248',
})
</script>

<template>
  <div class="arc-orbit-loader" :class="{ 'is-active': active }" aria-hidden="true"></div>
</template>

<style scoped>
/* Static accent ring — deliberately NO animation. The 2026-08-28 A/B
   measurement showed the previous per-frame conic-gradient repaint (animated
   --arc-angle) cost ~11 GPU points during runs (peaks: 21% baseline vs 9.9%
   with the loader off) — it was the dominant residual. A transform-based spin
   is impossible without distorting the rounded-rectangle ring (only a circle
   can rotate without shape change), so the ring is painted once and the
   thinking-gap dots + label carry the activity affordance. */
.arc-orbit-loader {
  position: absolute;
  inset: 0;
  border-radius: v-bind(radius);
  pointer-events: none;
  opacity: 0;
  z-index: 1;
  transition: opacity 0.4s ease-out;
}
.arc-orbit-loader.is-active { opacity: 1; }
.arc-orbit-loader::before {
  content: "";
  position: absolute;
  inset: 0;
  border-radius: inherit;
  padding: v-bind(thickness + 'px');
  background: conic-gradient(
    from 0deg,
    transparent 0deg,
    transparent 240deg,
    rgba(v-bind(color), 1) 300deg,
    transparent 360deg
  );
  -webkit-mask:
    linear-gradient(#000 0 0) content-box,
    linear-gradient(#000 0 0);
  -webkit-mask-composite: xor;
          mask-composite: exclude;
}
</style>
