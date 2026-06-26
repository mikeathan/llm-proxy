<script setup lang="ts">
import { watch } from 'vue'
import MarkdownViewer from '../../common/MarkdownViewer.vue'
import type { Segment } from '../../../types/assistant'
import type { Turn } from '../../../utils/turnGrouper'
import Icon from '../../icons/Icon.vue'
import ArcOrbitLoader from '../../common/ArcOrbitLoader.vue'
import ToolCallSegment from './ToolCallSegment.vue'
import { segKey } from '../../../composables/useToolDisplay'
import { useElapsedTimer } from '../../../composables/useElapsedTimer'

defineOptions({ inheritAttrs: false })

const props = defineProps<{
  turn: Turn
  idx: number
  loading: boolean
  thinking: boolean
  liveReasoning: string
  paused: boolean
  isLastTurn: boolean
  isWorkCollapsed: boolean
  isSegExpanded: (turnIdx: number, segIdx: number) => boolean
}>()

const emit = defineEmits<{
  toggleWork: [idx: number]
  toggleSegment: [turnIdx: number, segIdx: number]
}>()

const { seconds, start, stop } = useElapsedTimer()
watch(() => props.loading && props.isLastTurn, (active) => {
  if (active) start()
  else stop()
}, { immediate: true })

function toolStepCount(segments: Segment[]): number {
  return segments.filter(s => s.kind === 'tool_call').length
}
</script>

<template>
  <div
    :class="['message-wrapper', 'message-wrapper--assistant', { 'is-loading': loading && isLastTurn }]"
  >
    <div class="message-bubble message-bubble--assistant">
      <ArcOrbitLoader v-if="!turn.canceled" :active="loading && isLastTurn" radius="1rem" />

      <button
        v-if="!turn.canceled || turn.segments.length > 0"
        class="bubble-header"
        @click="emit('toggleWork', idx)"
        :class="{ 'bubble-header--clickable': turn.segments.length > 0 }"
      >
        <span class="bubble-header-label">Assistant</span>
        <span v-if="turn.segments.length > 0 || (loading && isLastTurn)" class="bubble-header-summary">
          <template v-if="loading && isLastTurn">
            {{ toolStepCount(turn.segments) > 0 ? toolStepCount(turn.segments) + ' step' + (toolStepCount(turn.segments) !== 1 ? 's' : '') + ' ' : '' }}in progress · {{ seconds }}s
          </template>
          <template v-else-if="turn.segments.length > 0">{{ toolStepCount(turn.segments) }} step{{ toolStepCount(turn.segments) !== 1 ? 's' : '' }} completed<span v-if="seconds > 0"> · {{ seconds }}s</span></template>
          <template v-else>completed<span v-if="seconds > 0"> · {{ seconds }}s</span></template>
        </span>
        <span v-if="turn.segments.length > 0" class="bubble-header-chevron">
          <Icon :name="isWorkCollapsed ? 'chevron-right' : 'chevron-down'" size="xs" />
        </span>
      </button>

      <div v-if="(turn.segments.length > 0 || (loading && thinking && liveReasoning)) && !isWorkCollapsed" class="bubble-work-section">
        <div
          v-for="(seg, sIdx) in turn.segments"
          :key="`seg-${idx}-${sIdx}`"
          :data-seg-key="segKey(idx, sIdx)"
        >
          <MarkdownViewer v-if="seg.kind === 'reasoning'" :content="seg.text" class="bubble-reasoning" />
          <ToolCallSegment
            v-if="seg.kind === 'tool_call'"
            :segment="seg"
            :turn-idx="idx"
            :seg-idx="sIdx"
            :expanded="isSegExpanded(idx, sIdx)"
            @toggle="(turnIdx, segIdx) => emit('toggleSegment', turnIdx, segIdx)"
          />
        </div>
        <MarkdownViewer
          v-if="loading && isLastTurn && thinking && liveReasoning"
          :content="liveReasoning"
          class="bubble-reasoning"
        />
      </div>

      <div
        v-if="!turn.canceled"
        :class="{ 'thinking-gap-hidden': !(loading && isLastTurn && paused && !turn.finalAnswer) }"
        class="thinking-gap"
      >
        <span class="thinking-gap-dot"></span>
        <span class="thinking-gap-dot"></span>
        <span class="thinking-gap-dot"></span>
        <span class="thinking-gap-text">&nbsp;Thinking</span>
      </div>

      <div v-if="turn.canceled" class="bubble-canceled-banner">
        <Icon name="close" size="xs" />
        <span>Response interrupted — send a new message to continue</span>
      </div>

      <div v-if="turn.finalAnswer" class="bubble-result-section">
        <div class="bubble-result-label">Result</div>
        <MarkdownViewer :content="turn.finalAnswer" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.message-wrapper { @apply flex w-full; }
.message-wrapper--assistant { @apply justify-start; }

.message-bubble { @apply max-w-full sm:max-w-[85%] rounded-2xl p-3 sm:p-4 flex flex-col gap-2 shadow-sm relative break-words; isolation: isolate; }
.message-bubble--assistant { @apply bg-gray-800 text-gray-200 border border-gray-700; min-height: 60px; }
.message-bubble--assistant > :not(.arc-orbit-loader) { position: relative; z-index: 0; }

.bubble-header { @apply flex items-center gap-2 select-none w-full; background: none; border: none; padding: 0; text-align: left; font-family: inherit; color: inherit; font-size: inherit; cursor: pointer; }
.bubble-header:hover .bubble-header-chevron { color: #d1d5db; }
.bubble-header-label { @apply text-[10px] font-bold uppercase tracking-wider text-gray-400; }
.bubble-header-summary { @apply text-[10px] text-gray-500 font-normal flex-1 truncate; }
.bubble-header-chevron { @apply text-gray-500 w-3 h-3 flex items-center justify-center; }

.bubble-work-section { @apply flex flex-col gap-1 pt-1; min-height: 20px; }
.bubble-reasoning { @apply text-xs leading-relaxed text-gray-300 px-1 pb-1; }

.thinking-gap { display: flex; align-items: center; gap: 4px; padding: 4px 8px; font-size: 11px; color: rgb(107, 114, 128); }
.thinking-gap-hidden { visibility: hidden; }
.thinking-gap-dot { width: 5px; height: 5px; border-radius: 50%; background: rgb(107, 114, 128); animation: thinking-pulse 1.2s ease-in-out infinite; }
.thinking-gap-dot:nth-child(2) { animation-delay: 0.2s; }
.thinking-gap-dot:nth-child(3) { animation-delay: 0.4s; }
.thinking-gap-text { color: rgb(107, 114, 128); font-size: 11px; }
@keyframes thinking-pulse { 0%, 60%, 100% { opacity: 0.3; } 30% { opacity: 1; } }

.bubble-result-section { @apply pt-2; }
.bubble-result-label { @apply text-[10px] font-bold uppercase tracking-wider text-blue-400 mb-2; }

.bubble-canceled-banner { display: flex; align-items: center; gap: 6px; padding: 6px 8px; margin: 4px 0 0; border-radius: 6px; font-size: 11px; color: #94a3b8; background: rgba(239, 68, 68, 0.08); border: 1px solid rgba(239, 68, 68, 0.2); }
</style>
