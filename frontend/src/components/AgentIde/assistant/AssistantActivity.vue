<script setup lang="ts">
import { ref, computed, onUnmounted, watch } from 'vue'
import type { SessionBrief } from '../../../types/assistant'
import { sourceIcon } from '../../../utils/assistant/source'
import { formatElapsedSince } from '../../../utils/format/time'
import Icon from '../../icons/Icon.vue'
import MicroLabel from '../../common/display/MicroLabel.vue'

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
  <section aria-label="Assistant activity" class="flex flex-col">
    <div class="flex items-center justify-between gap-2 border-b border-hairline px-4 py-3">
      <span class="flex items-center gap-3">
        <MicroLabel>Assistant activity</MicroLabel>
        <span class="font-mono text-[length:var(--text-micro)] tabular-nums text-muted">{{ sorted.length }} running</span>
      </span>
      <span v-if="loading" role="status" class="font-mono text-[length:var(--text-micro)] text-faint">Updating…</span>
    </div>

    <div class="p-4">
      <p v-if="sorted.length === 0" class="m-0 py-4 text-center text-[length:var(--text-small)] text-faint">No assistant runs active</p>

      <ul v-else class="m-0 flex list-none flex-col gap-2 p-0">
        <li v-for="s in sorted" :key="s.id">
          <button
            type="button"
            class="flex w-full min-w-0 flex-col gap-1 rounded-[var(--radius-sm)] border border-hairline border-l-2 border-l-state-live bg-canvas p-3 text-left transition-colors hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2"
            @click="emit('select-session', s.id)"
          >
            <span class="flex min-w-0 items-center gap-1.5">
              <Icon v-if="sourceIcon(s.source)" :name="sourceIcon(s.source)!" size="xs" class-name="flex-none text-muted" />
              <span class="min-w-0 flex-1 truncate text-[length:var(--text-small)] text-primary">{{ s.snippet || 'Assistant run' }}</span>
              <span aria-hidden="true" class="live-dot h-1.5 w-1.5 flex-none rounded-full"></span>
            </span>
            <span class="font-mono text-[length:var(--text-micro)] text-faint">{{ s.id.slice(-6) }} · running {{ elapsed(s) }}</span>
          </button>
        </li>
      </ul>
    </div>
  </section>
</template>

<style scoped>
.live-dot {
  background: rgb(var(--state-live));
}
</style>
