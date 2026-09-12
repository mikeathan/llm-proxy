<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { AdminApiService } from '../../services/admin/adminService'
import { useToast } from '../../composables/useToast'
import { errorMessage } from '../../utils/errors'
import { formatTime } from '../../utils/format/time'
import type { TerminalSessionView } from '../../types/admin'

const toast = useToast()

const sessions = ref<TerminalSessionView[]>([])
const isLoading = ref(true)
const loadError = ref('')
let pollTimer: ReturnType<typeof setInterval> | null = null
// Request sequencing: discard a stale response that resolves after a newer
// poll started (prevents an out-of-order write from a slow request).
let sessionsReqId = 0

const fetchSessions = async () => {
  const mine = ++sessionsReqId
  try {
    const data = await AdminApiService.fetchTerminalSessions()
    if (mine !== sessionsReqId) return
    sessions.value = data
    loadError.value = ''
  } catch (e) {
    if (mine !== sessionsReqId) return
    loadError.value = errorMessage(e)
  } finally {
    if (mine === sessionsReqId) isLoading.value = false
  }
}

const resetTerminal = async (workspaceID: string) => {
  try {
    await AdminApiService.resetTerminalSession(workspaceID)
    toast.success(`Terminal session reset for ${workspaceID}`)
    await fetchSessions()
  } catch (e) {
    toast.error(`Failed to reset terminal: ${errorMessage(e)}`)
  }
}

onMounted(() => {
  void fetchSessions()
  pollTimer = setInterval(() => void fetchSessions(), 5000)
})

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
  pollTimer = null
})
</script>

<template>
  <div class="monitor-card">
    <div class="monitor-header">
      <div class="flex items-center gap-2">
        <span class="pulse-icon" :class="{ 'active': sessions.length > 0 }"></span>
        <h3 class="monitor-title">Active Host Terminals</h3>
      </div>
      <span class="session-count">{{ sessions.length }} active</span>
    </div>

    <div v-if="isLoading && sessions.length === 0" class="p-8 text-center text-gray-500">
      Initializing monitor...
    </div>

    <div v-else-if="loadError && sessions.length === 0" class="empty-state">
      Unable to load terminal sessions: {{ loadError }}.
      <button class="retry-btn" @click="fetchSessions">Retry</button>
    </div>

    <div v-else-if="sessions.length === 0" class="empty-state">
      No active persistent sessions. Terminals are spawned on-demand.
    </div>

    <div v-else class="session-list">
      <div
        v-for="s in sessions"
        :key="`${s.workspace_id}|${s.network_on}`"
        class="session-item"
      >
        <div class="session-info">
          <div class="workspace-id">
            {{ s.workspace_id }}
            <span class="scope-tag" :class="s.network_on ? 'scope-on' : 'scope-off'">
              {{ s.network_on ? 'net on' : 'net off' }}
            </span>
          </div>
          <div class="session-meta">
            <span>Last activity: {{ formatTime(s.last_used) }}</span>
            <span class="separator">•</span>
            <span class="path-truncate" :title="s.host_path">{{ s.host_path }}</span>
          </div>
        </div>

        <button
          class="reset-btn"
          type="button"
          title="Force kill and reset all sessions for this workspace"
          @click="resetTerminal(s.workspace_id)"
        >
          Reset
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.monitor-card {
  @apply bg-gray-950/50 border border-gray-800 rounded-lg overflow-hidden mt-6;
}

.monitor-header {
  @apply p-4 border-b border-gray-800 flex items-center justify-between bg-gray-900/30;
}

.monitor-title {
  @apply text-sm font-medium text-gray-300;
}

.session-count {
  @apply text-xs font-mono text-blue-400 bg-blue-400/10 px-2 py-0.5 rounded-full;
}

.empty-state {
  @apply p-8 text-center text-sm text-gray-500 font-light italic flex flex-col items-center gap-2;
}

.retry-btn {
  @apply text-xs text-blue-400 hover:text-blue-300 underline;
}

.session-list {
  @apply divide-y divide-gray-800/50 max-h-64 overflow-y-auto;
}

.session-item {
  @apply p-4 flex items-center justify-between hover:bg-gray-800/20 transition-colors;
}

.session-info {
  @apply flex-1 min-w-0 pr-4;
}

.workspace-id {
  @apply text-sm font-medium text-gray-200 truncate flex items-center gap-2;
}

.scope-tag {
  @apply text-[10px] font-mono uppercase tracking-wide px-1.5 py-0.5 rounded-full shrink-0;
}
.scope-on {
  @apply bg-emerald-900/40 text-emerald-300 border border-emerald-700/50;
}
.scope-off {
  @apply bg-gray-800 text-gray-400 border border-gray-700;
}

.session-meta {
  @apply flex items-center gap-2 text-xs text-gray-500 mt-1;
}

.separator {
  @apply opacity-30;
}

.path-truncate {
  @apply truncate block max-w-xs;
}

.reset-btn {
  @apply text-xs font-medium text-red-400 hover:text-red-300 bg-red-400/10 hover:bg-red-400/20 px-3 py-1.5 rounded transition-all active:scale-95;
}

.pulse-icon {
  @apply w-2 h-2 rounded-full bg-gray-600 transition-colors;
}

.pulse-icon.active {
  @apply bg-blue-500 shadow-[0_0_8px_rgba(59,130,246,0.5)];
  animation: pulse 2s cubic-bezier(0.4, 0, 0.6, 1) infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: .5; }
}
</style>
