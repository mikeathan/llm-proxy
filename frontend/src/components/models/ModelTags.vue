<script setup lang="ts">
import type { ModelMetadata } from '../../types'
import { formatParameters } from '../../utils/model-discovery'

defineProps<{
  metadata?: ModelMetadata
}>()
</script>

<template>
  <div v-if="metadata" class="model-tags-container">
    <span class="tag-item param-tag" v-if="metadata.parameters">
      {{ formatParameters(metadata.parameters) }}
    </span>
    <span class="tag-item quant-tag" v-if="metadata.quantization">
      {{ metadata.quantization }}
    </span>
    <span class="tag-item ctx-tag" v-if="metadata.context_length">
      {{ Math.round(metadata.context_length / 1024) }}K
    </span>
    <span class="tag-item arch-tag" v-if="metadata.architecture">
      {{ metadata.architecture }}
    </span>
  </div>
</template>

<style scoped lang="postcss">
.model-tags-container {
  @apply flex items-center gap-1.5;
}

.tag-item {
  @apply text-[12px] font-black px-2 py-0.5 rounded border leading-none uppercase tracking-tighter;
}

.param-tag {
  @apply bg-purple-500/10 text-purple-400 border-purple-500/20;
}

.quant-tag {
  @apply bg-blue-500/10 text-blue-400 border-blue-500/20;
}

.ctx-tag {
  @apply bg-gray-500/10 text-gray-400 border-gray-500/20;
}

.arch-tag {
  @apply bg-teal-500/10 text-teal-400 border-teal-500/20;
}
</style>
