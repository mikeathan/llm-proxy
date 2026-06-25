<script setup lang="ts">
import { computed } from "vue";
import { getPhaseMessage } from "../../constants/icons";

const props = defineProps<{
  phase: string;
  payload: Record<string, any>;
}>();

const text = computed(() => getPhaseMessage(props.phase, props.payload));
</script>

<template>
  <div v-if="text" class="lifecycle-message">
    <span class="lifecycle-text">{{ text }}</span>
  </div>
</template>

<style scoped lang="postcss">
.lifecycle-message {
  @apply border-l-2 border-indigo-500/30 pl-2 bg-indigo-500/5 py-1 my-1 text-indigo-200/60 italic text-[11px];
}
</style>
