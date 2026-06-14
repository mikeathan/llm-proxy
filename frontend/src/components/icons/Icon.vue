<script setup lang="ts">
import { defineAsyncComponent, computed } from "vue";

const props = withDefaults(defineProps<{
  name: string;
  size?: "xs" | "sm" | "md" | "lg";
  className?: string;
}>(), {
  size: "md",
});

const sizeClass = computed(() => {
  switch (props.size) {
    case 'xs': return 'h-3 w-3';
    case 'sm': return 'h-4 w-4';
    case 'lg': return 'h-6 w-6';
    default: return 'h-5 w-5';
  }
});

const iconComponent = computed(() => {
  if (props.name === 'spinner') return null; // handled in template
  return defineAsyncComponent(() => import(`../../assets/svg/${props.name}.svg?component`));
});
</script>

<template>
  <div v-if="name === 'spinner'" :class="[sizeClass, className, 'animate-spin rounded-full border-b-2 border-current']"></div>
  <component v-else :is="iconComponent" :class="[sizeClass, className]" />
</template>
