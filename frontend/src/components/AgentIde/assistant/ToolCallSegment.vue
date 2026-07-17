<script setup lang="ts">
import { ref, computed } from 'vue'
import Icon from '../../icons/Icon.vue'
import { toolLabel, toolIconClass } from '../../../composables/assistant/useToolDisplay'

interface ToolCallSeg {
  kind: 'tool_call'
  name: string
  args: string
  status: string
  result?: string
  error?: string
}

const props = defineProps<{
  segment: ToolCallSeg
  turnIdx: number
  segIdx: number
  expanded: boolean
  compact?: boolean
}>()

const emit = defineEmits<{
  toggle: [turnIdx: number, segIdx: number]
}>()

// In compact mode the row is collapsed by default; clicking toggles a local
// inline expansion. In non-compact mode the `expanded` prop drives detail.
const compactExpanded = ref(false)

const showDetail = computed(() => {
  if (props.compact) return compactExpanded.value
  return props.expanded
})

function onClick() {
  if (props.compact) {
    compactExpanded.value = !compactExpanded.value
  } else {
    emit('toggle', props.turnIdx, props.segIdx)
  }
}
</script>

<template>
  <div class="segment-item">
    <button
      class="segment-header"
      :class="['segment-item--tool', toolIconClass(segment.status || ''), { 'segment-item--running': segment.status === 'running' }, { 'segment-item--compact': compact }]"
      @click="onClick"
    >
      <span class="segment-icon">
        <Icon v-if="segment.status === 'running'" name="spinner" size="xs" />
        <Icon v-else-if="segment.status === 'success'" name="check" size="xs" />
        <Icon v-else name="close" size="xs" />
      </span>
      <span class="segment-label">
        <span class="segment-name">{{ toolLabel(segment.name, segment.args || '') }}</span>
        <span v-if="compact && segment.result" class="segment-preview">
          → {{ segment.result.length > 48 ? segment.result.slice(0, 48) + '…' : segment.result }}
        </span>
        <span v-else-if="compact && segment.error" class="segment-preview segment-preview--error">
          → {{ segment.error.length > 48 ? segment.error.slice(0, 48) + '…' : segment.error }}
        </span>
      </span>
      <span class="segment-chevron">
        <Icon :name="showDetail ? 'chevron-down' : 'chevron-right'" size="xs" />
      </span>
    </button>

    <div v-if="showDetail" class="segment-detail">
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
.segment-detail { @apply mt-1 ml-6 p-3 bg-gray-800/40 border border-white/5 rounded-2xl flex flex-col gap-2; }
.segment-detail-row { @apply flex flex-col gap-1; }
.segment-detail-key { @apply text-[10px] uppercase tracking-wider text-gray-500 font-semibold; }
.segment-detail-value { @apply text-xs font-mono text-gray-300 whitespace-pre-wrap break-words max-h-48 overflow-y-auto; }

/* ── Compact mode (inside inset) ── */
.segment-item--compact { @apply py-0.5; }
.segment-label { @apply flex-1 font-mono text-gray-400 truncate flex items-center gap-2; }
.segment-preview { @apply text-gray-500 text-[10px] truncate flex-1; }
.segment-preview--error { @apply text-red-400/70; }
</style>
