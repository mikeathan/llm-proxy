<script setup lang="ts">
withDefaults(defineProps<{
  active: boolean
  radius?: string
  thickness?: number
  duration?: number
  color?: string
  glow?: number
}>(), {
  radius: '1rem',
  thickness: 1.5,
  duration: 2.4,
  color: '129, 140, 248',
  glow: 4,
})
</script>

<template>
  <div class="arc-orbit-loader" :class="{ 'is-active': active }" aria-hidden="true"></div>
</template>

<style scoped>
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
    from var(--arc-angle, 0deg),
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
  animation: arc-orbit v-bind(duration + 's') linear infinite;
  filter: drop-shadow(0 0 v-bind(glow + 'px') rgba(v-bind(color), 0.45));
}
@keyframes arc-orbit {
  to { --arc-angle: 360deg; }
}
@property --arc-angle {
  syntax: '<angle>';
  inherits: false;
  initial-value: 0deg;
}
@media (prefers-reduced-motion: reduce) {
  .arc-orbit-loader.is-active::before {
    animation: none;
    background: rgba(v-bind(color), 0.5);
  }
}
</style>
