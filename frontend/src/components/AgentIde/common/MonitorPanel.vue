<script setup lang="ts">
import type { AutomationRun, DispatcherMetrics } from "../../../types/dispatcher"
import type { SessionBrief } from "../../../types/assistant"
import WorkspaceActivity from "../workspace/WorkspaceActivity.vue"
import SystemMetricsPanel from "../system/SystemMetricsPanel.vue"
import AssistantActivity from "../assistant/AssistantActivity.vue"

// Monitor content for the context drawer (formerly the permanent right rail):
// running assistant sessions (Workspaces only), run history and dispatcher
// metrics. Host metrics moved to the Overview.

defineProps<{
  history: AutomationRun[]
  loading: boolean
  metrics: DispatcherMetrics | null
  // Present in Workspaces, where the assistant runs; omitted in Automations.
  assistantSessions?: SessionBrief[]
  // Label of the run occupying the lane while a chat waits for a slot.
  laneWaitingLabel?: string
}>()

const emit = defineEmits<{
  (e: "select-run", run: AutomationRun): void
  (e: "select-assistant-session", sessionId: string): void
  (e: "delete-run", run: AutomationRun): void
}>()
</script>

<template>
  <div class="flex flex-col gap-4">
    <div v-if="assistantSessions" data-test="assistant-activity" class="monitor-card">
      <AssistantActivity
        :sessions="assistantSessions"
        @select-session="(id: string) => emit('select-assistant-session', id)"
      />
      <p v-if="laneWaitingLabel" class="px-3 pb-3 text-center font-mono text-[length:var(--text-micro)] text-muted">
        Waiting for {{ laneWaitingLabel }} to finish
      </p>
    </div>

    <div class="monitor-card">
      <WorkspaceActivity
        :history="history"
        :loading="loading"
        @select-run="(run: AutomationRun) => emit('select-run', run)"
        @delete-run="(run: AutomationRun) => emit('delete-run', run)"
      />
    </div>

    <SystemMetricsPanel :metrics="metrics" />
  </div>
</template>

<style scoped lang="postcss">
.monitor-card {
  @apply flex flex-col overflow-hidden rounded-[var(--radius-sm)] border border-hairline bg-surface;
}
</style>
