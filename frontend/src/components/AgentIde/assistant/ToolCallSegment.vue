<script setup lang="ts">
import Icon from '../../icons/Icon.vue'
import { toolLabel, toolIconClass } from '../../../composables/useToolDisplay'

interface ToolCallSeg {
  kind: 'tool_call'
  name: string
  args: string
  status: string
  result?: string
  error?: string
}

defineProps<{
  segment: ToolCallSeg
  turnIdx: number
  segIdx: number
  expanded: boolean
}>()

const emit = defineEmits<{
  toggle: [turnIdx: number, segIdx: number]
}>()
</script>

<template>
  <div class="segment-item">
    <button
      class="segment-header"
      :class="['segment-item--tool', toolIconClass(segment.status || ''), { 'segment-item--running': segment.status === 'running' }]"
      @click="emit('toggle', turnIdx, segIdx)"
    >
      <span class="segment-icon">
        <Icon v-if="segment.status === 'running'" name="spinner" size="xs" />
        <Icon v-else-if="segment.status === 'success'" name="check" size="xs" />
        <Icon v-else name="close" size="xs" />
      </span>
      <span class="segment-label">
        <span class="segment-name">{{ toolLabel(segment.name, segment.args || '') }}</span>
      </span>
      <span class="segment-chevron">
        <Icon :name="expanded ? 'chevron-down' : 'chevron-right'" size="xs" />
      </span>
    </button>

    <div v-if="expanded" class="segment-detail">
      <div class="segment-detail-row">
        <span class="segment-detail-key">Args</span>
        <pre class="segment-detail-value">{{ segment.args }}</pre>
      </div>
      <div v-if="segment.result" class="segment-detail-row">
        <span class="segment-detail-key">Result</span>
        <pre class="segment-detail-value">{{ segment.result }}</pre>
      </div>
      <div v-if="segment.error" class="segment-detail-row">
        <span class="segment-detail-key">Error</span>
        <pre class="segment-detail-value">{{ segment.error }}</pre>
      </div>
    </div>
  </div>
</template>

<style scoped>
.segment-item { @apply text-[11px]; }
.segment-header { @apply w-full flex items-center gap-2 px-1.5 py-1 rounded hover:bg-white/5 transition-colors text-left; border: none; cursor: pointer; font-family: inherit; font-size: inherit; color: inherit; background: transparent; }
.segment-icon { @apply flex items-center justify-center w-3.5 h-3.5 shrink-0; }
.segment-icon:deep(.h-4\\.w-4) { @apply h-3.5 w-3.5; }
.segment-icon:deep(svg) { @apply w-3.5 h-3.5; }
.segment-item--running .segment-icon { @apply text-blue-400; }
.segment-header:not(.segment-item--running):not(.segment-item--error) .segment-icon { @apply text-green-500/70; }
.segment-item--error .segment-icon { @apply text-red-400/70; }
.segment-label { @apply flex-1 font-mono text-gray-500 truncate flex items-center gap-2; }
.segment-name { @apply shrink-0; }
.segment-chevron { @apply text-gray-600 w-3 h-3 flex items-center justify-center shrink-0; }
.segment-detail { @apply mt-1 ml-6 p-2 bg-gray-900/50 rounded border border-gray-700/40 flex flex-col gap-2; }
.segment-detail-row { @apply flex flex-col gap-1; }
.segment-detail-key { @apply text-[10px] uppercase tracking-wider text-gray-500 font-semibold; }
.segment-detail-value { @apply text-xs font-mono text-gray-300 whitespace-pre-wrap break-words max-h-48 overflow-y-auto; }
</style>
