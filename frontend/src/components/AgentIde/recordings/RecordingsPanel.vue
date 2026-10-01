<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import type { RecordingMeta, Automation } from '../../../types/dispatcher'
import { useRecordings } from '../../../composables/automation/useRecordings'
import { useConfirm } from '../../../composables/ui/useConfirm'
import { formatBytes } from '../../../utils/format/formatters'
import { formatAbsoluteTime } from '../../../utils/format/time'
import StatusTag from '../../common/display/StatusTag.vue'
import BaseButton from '../../common/buttons/BaseButton.vue'
import LoadingState from '../../common/feedback/LoadingState.vue'
import EmptyState from '../../common/feedback/EmptyState.vue'
import Icon from '../../icons/Icon.vue'

// Recorded LLM sessions, per automation: replay one instead of calling the
// model (one replay at a time), switch an automation back to the live model,
// or delete a recording.

const props = defineProps<{
  automations: Automation[]
  workspaces: string[]
}>()

const emit = defineEmits<{
  (e: 'recording-deselected', recording: RecordingMeta): void
  (e: 'replay-recording', auto: Automation, recording: RecordingMeta): void
  (e: 'stop-automation', workspace: string): void
  (e: 'show-automation', id: string): void
}>()

const {
  recordings,
  status,
  loading,
  fetchStatus,
  fetchRecordings,
  deleteRecording,
  clearAutomationRecordingRef,
} = useRecordings()
const { confirm } = useConfirm()

const replayingId = ref<string | null>(null)

// A replay ends when nothing is running any more.
watch(() => props.automations, (autos) => {
  if (replayingId.value && !autos.some(a => a.is_running)) {
    replayingId.value = null
  }
}, { deep: true })

const expandedAuto = ref<string | null>(null)

onMounted(async () => {
  await fetchStatus()
  if (status.value.enabled) {
    await fetchRecordings()
  }
})

function recordingsForAuto(automationName: string): RecordingMeta[] {
  return recordings.value.filter(r => r.automation_name === automationName)
    .sort((a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime())
}

const toggleExpand = (name: string) => {
  expandedAuto.value = expandedAuto.value === name ? null : name
}

const isLive = (auto: Automation, rec: RecordingMeta) => rec.id === replayingId.value && !!auto.is_running

async function clearRecording(auto: Automation, recording: RecordingMeta) {
  if (!auto.workspace) return
  await clearAutomationRecordingRef(auto.workspace, auto.name)
  emit('recording-deselected', recording)
}

async function deleteRecordingEntry(rec: RecordingMeta) {
  const ok = await confirm({
    title: 'Delete this recording?',
    message: `The recording from ${formatAbsoluteTime(rec.timestamp)} is removed. This cannot be undone.`,
    type: 'error',
    confirmText: 'Delete recording',
  })
  if (ok) await deleteRecording(rec.id)
}

function handleReplayClick(auto: Automation, rec: RecordingMeta) {
  replayingId.value = rec.id
  emit('replay-recording', auto, rec)
}
</script>

<template>
  <div class="flex flex-col gap-3">
    <p class="m-0 flex flex-wrap items-center gap-2">
      <StatusTag :state="status.enabled ? 'success' : 'neutral'" :label="status.enabled ? 'Recording on' : 'Recording off'" />
      <span v-if="status.enabled" class="break-all font-mono text-[length:var(--text-small)] text-muted">{{ status.dir }}</span>
      <span v-else class="text-[length:var(--text-small)] text-muted">Start the server with <code class="font-mono">--record</code> to capture runs.</span>
    </p>

    <LoadingState v-if="loading" label="Loading recordings" :rows="3" />
    <EmptyState
      v-else-if="recordings.length === 0"
      title="No recordings yet"
      body="Run automations while recording is on; each run's model traffic is saved here for replay."
    />

    <ul v-else class="m-0 flex list-none flex-col p-0">
      <li v-for="auto in automations" :key="auto.id" class="border-b border-hairline last:border-b-0">
        <button
          type="button"
          data-test="recording-group"
          :aria-expanded="expandedAuto === auto.name"
          class="flex w-full items-center gap-2 px-1 py-2.5 text-left hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2"
          @click="toggleExpand(auto.name)"
        >
          <Icon name="chevron-right" size="xs" :class-name="['flex-none text-muted transition-transform duration-fast', expandedAuto === auto.name ? 'rotate-90' : ''].join(' ')" />
          <span class="min-w-0 flex-1 truncate text-primary">{{ auto.name }}</span>
          <StatusTag v-if="auto.recording_ref" state="info" label="Replays a recording" />
          <span class="font-mono text-[length:var(--text-micro)] text-muted">{{ auto.workspace }}</span>
        </button>

        <div v-if="expandedAuto === auto.name" class="pb-3 pl-6">
          <p v-if="recordingsForAuto(auto.name).length === 0" class="m-0 text-[length:var(--text-small)] text-muted">No recordings for this automation.</p>
          <ul v-else class="m-0 flex list-none flex-col gap-1 p-0">
            <li
              v-for="r in recordingsForAuto(auto.name)"
              :key="r.id"
              data-test="recording"
              :class="['flex flex-wrap items-center gap-2 rounded-[var(--radius-sm)] px-2 py-1.5', auto.recording_ref === r.id ? 'bg-surface-active' : '']"
            >
              <span class="flex min-w-0 flex-1 flex-col">
                <span class="font-mono text-[length:var(--text-small)] text-primary">{{ formatAbsoluteTime(r.timestamp) }}</span>
                <span class="font-mono text-[length:var(--text-micro)] text-muted">{{ r.model }} · {{ formatBytes(r.file_size) }}</span>
              </span>
              <template v-if="isLive(auto, r)">
                <button type="button" class="focus-visible:outline-none focus-visible:ring-2" @click="emit('show-automation', auto.id)">
                  <StatusTag state="running" label="Replaying" />
                </button>
                <BaseButton variant="danger" size="sm" icon="stop" @click="emit('stop-automation', auto.workspace)">Stop</BaseButton>
              </template>
              <BaseButton v-else-if="auto.recording_ref === r.id" variant="secondary" size="sm" @click="clearRecording(auto, r)">Use live model</BaseButton>
              <BaseButton v-else variant="secondary" size="sm" icon="play" :disabled="replayingId !== null" @click="handleReplayClick(auto, r)">Replay</BaseButton>
              <BaseButton
                variant="ghost"
                size="sm"
                icon="trash"
                icon-only
                :label="`Delete recording from ${formatAbsoluteTime(r.timestamp)}`"
                :disabled="isLive(auto, r)"
                @click="deleteRecordingEntry(r)"
              />
            </li>
          </ul>
        </div>
      </li>
    </ul>
  </div>
</template>
