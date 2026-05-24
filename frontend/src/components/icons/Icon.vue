<script setup lang="ts">
import { computed } from "vue";
import { SVG_ICONS, type SvgIconName } from "../../assets/svg";

const props = withDefaults(defineProps<{
  name: SvgIconName;
  size?: "sm" | "md" | "lg";
}>(), {
  size: "sm",
});

const sizeClass = computed(() => ({
  sm: "w-3 h-3",
  md: "w-4 h-4",
  lg: "w-5 h-5",
}[props.size]));
</script>

<template>
  <span :class="['inline-flex items-center justify-center', sizeClass]" v-html="SVG_ICONS[name]" />
</template>
