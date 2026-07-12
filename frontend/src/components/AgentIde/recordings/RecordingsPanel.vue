<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import type { RecordingMeta, RecordingStatus, Automation } from '../../../types/dispatcher'
import { DispatcherService } from '../../../services/automation/dispatcherService'
import { formatBytes, formatTS } from '../../../utils/format/formatters'

const props = defineProps<{
  automations: Automation[]
  workspaces: string[]
}>()

const emit = defineEmits<{
  (e: 'recording-selected', recording: RecordingMeta): void
  (e: 'recording-deselected', recording: RecordingMeta): void
  (e: 'replay-recording', auto: Automation, recording: RecordingMeta): void
  (e: 'stop-automation', workspace: string): void
  (e: 'show-automation', id: string): void
}>()

const replayingId = ref<string | null>(null)
watch(() => props.automations, (autos) => {
  if (replayingId.value && !autos.some(a => a.is_running)) {
    replayingId.value = null
  }
}, { deep: true })

const recordings = ref<RecordingMeta[]>([])
const status = ref<RecordingStatus>({ enabled: false, dir: '' })
const loading = ref(false)
const selectedRecording = ref<string | null>(null)
const expandedAuto = ref<string | null>(null)

onMounted(async () => {
  try {
    status.value = await DispatcherService.getRecordingStatus()
    if (status.value.enabled) {
      await fetchRecordings()
    }
  } catch {
    status.value = { enabled: false, dir: '' }
  }
})

async function fetchRecordings() {
  loading.value = true
  try {
    recordings.value = await DispatcherService.listRecordings()
  } catch {
    recordings.value = []
  } finally {
    loading.value = false
  }
}

function recordingsForAuto(automationName: string): RecordingMeta[] {
  return recordings.value.filter(r => r.automation_name === automationName)
    .sort((a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime())
}

const toggleExpand = (name: string) => {
  expandedAuto.value = expandedAuto.value === name ? null : name
}

async function clearRecording(auto: Automation, recording: RecordingMeta) {
  if (!auto.workspace) return
  await DispatcherService.clearAutomationRecordingRef(auto.workspace, auto.name)
  selectedRecording.value = null
  emit('recording-deselected', recording)
}

async function deleteRecording(id: string) {
  await DispatcherService.deleteRecording(id)
  recordings.value = recordings.value.filter(r => r.id !== id)
}

function handleReplayClick(auto: Automation, rec: RecordingMeta) {
  replayingId.value = rec.id
  emit('replay-recording', auto, rec)
}
</script>

<template>
  <div class="panel-container">
    <!-- Recording Mode Status -->
    <div class="status-banner" :class="status.enabled ? 'status-banner--active' : 'status-banner--inactive'">
      <span class="status-dot" :class="status.enabled ? 'status-dot--on' : 'status-dot--off'"></span>
      {{ status.enabled ? `Recording ON (${status.dir})` : 'Recording OFF' }}
    </div>

    <!-- No recordings state -->
    <div v-if="!loading && recordings.length === 0" class="empty-state">
      <div class="empty-icon">
        <svg xmlns="http://www.w3.org/2000/svg" class="h-8 w-8" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M19 11a7 7 0 01-7 7m0 0a7 7 0 01-7-7m7 7v4m0 0H8m4 0h4m-4-8a3 3 0 01-3-3V5a3 3 0 116 0v6a3 3 0 01-3 3z" />
        </svg>
      </div>
      <div class="empty-text">No recordings found</div>
      <div class="empty-hint">Start server with <code>--record</code> and run automations to create recordings</div>
    </div>

    <!-- Recordings List -->
    <div v-for="auto in automations" :key="auto.id" class="recording-group">
      <div
        class="recording-group-header"
        :class="{ 'recording-group-header--expanded': expandedAuto === auto.name }"
        @click="toggleExpand(auto.name)"
      >
        <div class="recording-group-name">
          {{ auto.name }}
          <span v-if="auto.recording_ref" class="recording-badge">REC</span>
        </div>
        <div class="recording-group-meta">{{ auto.workspace }}</div>
      </div>

      <div v-if="expandedAuto === auto.name" class="recording-list">
        <div v-if="recordingsForAuto(auto.name).length === 0" class="recording-empty">
          No recordings for this automation
        </div>
        <div
          v-for="rec in recordingsForAuto(auto.name)"
          :key="rec.id"
          class="recording-item group"
          :class="{
            'recording-item--active': auto.recording_ref === rec.id,
            'recording-item--running': rec.id === replayingId && auto.is_running,
          }"
          @click="rec.id === replayingId && auto.is_running ? emit('show-automation', auto.id) : null"
        >
          <div class="recording-info">
            <div class="recording-time">
              <span v-if="rec.id === replayingId && auto.is_running" class="running-dot"></span>
              {{ formatTS(rec.timestamp) }}
            </div>
            <div class="recording-detail">
              {{ rec.model }} · {{ formatBytes(rec.file_size) }}
            </div>
          </div>
          <div class="recording-actions">
            <span
              v-if="rec.id === replayingId && auto.is_running"
              class="btn-running"
              title="Recording replay is running"
            >
              ● Live
            </span>
            <button
              v-else-if="auto.recording_ref === rec.id"
              @click.stop="clearRecording(auto, rec)"
              class="btn-playback btn-playback--active"
              title="Use live LLM instead"
            >
              Live
            </button>
            <button
              v-else
              @click.stop="handleReplayClick(auto, rec)"
              class="btn-replay"
              :class="{ 'btn-replay--disabled': replayingId !== null }"
              :disabled="replayingId !== null"
              title="Replay this recording now"
            >
              ▶ Replay
            </button>
            <button
              v-if="rec.id === replayingId && auto.is_running"
              @click.stop="emit('stop-automation', auto.workspace)"
              class="btn-stop"
              title="Stop running automation"
            >
              ■ Stop
            </button>
            <button
              @click.stop="deleteRecording(rec.id)"
              class="btn-delete"
              title="Delete recording"
              :disabled="rec.id === replayingId && auto.is_running"
            >
              ×
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.panel-container {
  @apply divide-y divide-gray-800;
}

.status-banner {
  @apply px-4 py-2 text-xs font-semibold flex items-center gap-2;
}
.status-banner--active {
  @apply text-green-400 bg-green-900/20;
}
.status-banner--inactive {
  @apply text-gray-500 bg-gray-800/30;
}
.status-dot {
  @apply w-2 h-2 rounded-full;
}
.status-dot--on {
  @apply bg-green-500;
}
.status-dot--off {
  @apply bg-gray-600;
}

.empty-state {
  @apply flex flex-col items-center py-8 px-4 text-center;
}
.empty-icon {
  @apply text-gray-600 mb-3;
}
.empty-text {
  @apply text-gray-400 text-sm font-medium mb-1;
}
.empty-hint {
  @apply text-gray-600 text-xs;
}
.empty-hint code {
  @apply text-gray-400 bg-gray-800 px-1 rounded;
}

.recording-group-header {
  @apply px-4 py-2.5 cursor-pointer transition-colors hover:bg-gray-700/50;
}
.recording-group-header--expanded {
  @apply bg-gray-700/30;
}
.recording-group-name {
  @apply text-sm font-medium text-gray-200 flex items-center gap-2;
}
.recording-badge {
  @apply text-[10px] font-bold text-orange-400 bg-orange-900/30 px-1.5 py-0.5 rounded;
}
.recording-group-meta {
  @apply text-xs text-gray-500 mt-0.5;
}

.recording-list {
  @apply border-t border-gray-800;
}
.recording-empty {
  @apply px-4 py-3 text-xs text-gray-600 italic;
}

.recording-item {
  @apply flex items-center px-4 py-2 gap-2 transition-colors hover:bg-gray-700/30;
}
.recording-item--active {
  @apply bg-orange-900/15;
}

.recording-info {
  @apply flex-1 min-w-0;
}
.recording-time {
  @apply text-xs text-gray-300;
}
.recording-detail {
  @apply text-[11px] text-gray-500;
}

.recording-actions {
  @apply flex items-center gap-1 shrink-0;
}

.recording-item--running {
  @apply bg-green-900/10 cursor-pointer;
}

.running-dot {
  @apply inline-block w-1.5 h-1.5 rounded-full bg-green-500 mr-1.5 shadow-[0_0_6px_rgba(34,197,94,0.6)] animate-pulse;
}

.btn-playback {
  @apply px-2 py-1 text-xs rounded transition-colors bg-gray-700 text-gray-400 hover:bg-blue-700 hover:text-white;
}
.btn-playback--active {
  @apply bg-orange-700 text-white hover:bg-red-700;
}
.btn-replay {
  @apply px-2 py-1 text-xs rounded transition-colors bg-emerald-700 text-emerald-200 hover:bg-emerald-600 hover:text-white;
}
.btn-replay--disabled {
  @apply opacity-40 cursor-not-allowed hover:bg-emerald-700 hover:text-emerald-200;
}
.btn-running {
  @apply px-2 py-1 text-xs rounded bg-green-800 text-green-300 font-bold cursor-default;
}
.btn-stop {
  @apply px-2 py-1 text-xs rounded transition-colors bg-red-800 text-red-300 hover:bg-red-700 hover:text-white font-bold;
}

.btn-delete {
  @apply px-1.5 py-1 text-xs text-gray-600 hover:text-red-400 transition-colors;
}
.btn-delete:disabled {
  @apply opacity-30 cursor-not-allowed hover:text-gray-600;
}
</style>
