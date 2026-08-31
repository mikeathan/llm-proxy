<script setup lang="ts">
import { ref, computed, onUnmounted, watch } from 'vue'
import type { SessionBrief } from '../../../types/assistant'
import { sourceIcon } from '../../../utils/assistant/source'
import { formatElapsedSince } from '../../../utils/format/time'
import Icon from '../../icons/Icon.vue'

const props = defineProps<{
  sessions: SessionBrief[]
  loading?: boolean
}>()

const emit = defineEmits<{
  (e: 'select-session', sessionId: string): void
}>()

const now = ref(Date.now())
let tick: ReturnType<typeof setInterval> | null = null

function startTick() {
  if (tick) return
  tick = setInterval(() => { now.value = Date.now() }, 1000)
}
function stopTick() {
  if (tick) { clearInterval(tick); tick = null }
}

watch(() => props.sessions.length, (n) => {
  if (n > 0) startTick()
  else stopTick()
}, { immediate: true })

onUnmounted(stopTick)

const sorted = computed(() =>
  [...props.sessions].sort(
    (a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime(),
  ),
)

const elapsed = (s: SessionBrief): string => formatElapsedSince(s.updated_at, now.value)
</script>

<template>
  <div class="activity-shell">
    <div class="activity-header">
      <div class="header-left">
        <h3 class="header-title">Assistant Activity</h3>
        <span class="pulse-count">{{ sorted.length }} running</span>
      </div>
      <div v-if="loading" class="loader-spinner"></div>
    </div>

    <div class="activity-content">
      <div v-if="sorted.length === 0" class="empty-activity">
        <p class="empty-text">No assistant runs active</p>
      </div>

      <div class="activity-list">
        <div
          v-for="s in sorted"
          :key="s.id"
          @click="emit('select-session', s.id)"
          class="session-card"
        >
          <div class="card-row card-row--top">
            <Icon v-if="sourceIcon(s.source)" :name="sourceIcon(s.source)!" size="xs" class="session-source" />
            <span class="session-snippet">{{ s.snippet || 'Assistant run' }}</span>
            <span class="pulse-dot"></span>
          </div>
          <div class="card-row card-row--bottom">
            <span class="meta-val">{{ s.id.slice(-6) }}</span>
            <span class="meta-sep">·</span>
            <span class="meta-val">running {{ elapsed(s) }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.activity-shell {
  @apply flex flex-col bg-gray-900/40;
}

.activity-header {
  @apply p-4 border-b border-gray-800 flex justify-between items-center bg-gray-900/60;
}

.header-left {
  @apply flex items-center gap-3;
}

.header-title {
  @apply text-[10px] font-bold text-gray-400 uppercase tracking-widest;
}

.pulse-count {
  @apply text-[8px] bg-gray-800 text-green-500 px-1.5 py-0.5 rounded font-mono;
}

.loader-spinner {
  @apply animate-spin h-3 w-3 border-2 border-blue-500 border-t-transparent rounded-full;
}

.activity-content {
  @apply p-4;
}

.empty-activity {
  @apply flex items-center justify-center py-6 opacity-30 italic text-[10px];
}

.activity-list {
  @apply space-y-3;
}

.session-card {
  @apply p-3 border-l-2 border-l-green-500 rounded-lg bg-gray-800/10 cursor-pointer hover:bg-gray-800/40 transition-colors border-gray-800/50 hover:border-gray-600;
}

.card-row {
  @apply flex justify-between items-center gap-2;
}
.card-row--top {
  @apply mb-1 min-w-0;
}
.card-row--bottom {
  @apply gap-1.5;
}

.session-source {
  @apply text-[11px] shrink-0;
}

.session-snippet {
  @apply text-xs text-gray-200 truncate font-medium flex-1 min-w-0;
}

.pulse-dot {
  @apply w-1.5 h-1.5 rounded-full shrink-0;
  background: var(--color-live, #22c55e);
}

.meta-val {
  @apply text-[9px] text-gray-500 font-mono;
}

.meta-sep {
  @apply text-gray-700 text-[8px];
}
</style>
