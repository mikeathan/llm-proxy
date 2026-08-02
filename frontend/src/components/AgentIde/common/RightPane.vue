<script setup lang="ts">
import type { AutomationRun, DispatcherMetrics } from "../../../types/dispatcher"
import type { ActiveModel } from "../../../types/model"
import type { SessionBrief } from "../../../types/assistant"
import MetricsPulse from "../../common/display/MetricsPulse.vue"
import BaseButton from "../../common/buttons/BaseButton.vue"
import WorkspaceActivity from "../workspace/WorkspaceActivity.vue"
import SystemMetricsPanel from "../system/SystemMetricsPanel.vue"
import AssistantActivity from "../assistant/AssistantActivity.vue"

defineProps<{
  systemMetrics: any
  activeModel: ActiveModel | null
  selectedAutomation: any
  anyRunningInSelectedWorkspace: boolean
  triggering: boolean
  workspaceHistory: AutomationRun[]
  assistantSessions: SessionBrief[]
  loading: boolean
  metrics: DispatcherMetrics | null
}>()

const emit = defineEmits<{
  (e: "trigger"): void
  (e: "stop"): void
  (e: "select-run", run: AutomationRun): void
  (e: "select-assistant-session", sessionId: string): void
}>()
</script>

<template>
  <div class="right-pane">
    <div class="pulse-container">
      <MetricsPulse :metrics="systemMetrics" :activeModel="activeModel" />
    </div>

    <div class="action-card">
      <h3 class="action-title">Actions</h3>
      <BaseButton
        v-if="!anyRunningInSelectedWorkspace"
        @click="emit('trigger')"
        variant="primary"
        icon="play"
        :loading="triggering"
        :disabled="!selectedAutomation || triggering"
        className="w-full"
      >
        Run Automation
      </BaseButton>
      <BaseButton
        v-else
        @click="emit('stop')"
        variant="danger"
        icon="stop"
        className="w-full"
      >
        Stop Automation
      </BaseButton>
      <p v-if="!selectedAutomation" class="action-helper">
        Select an automation to enable execution
      </p>
    </div>

    <div class="activity-container">
      <AssistantActivity
        :sessions="assistantSessions"
        @select-session="(id: string) => emit('select-assistant-session', id)"
      />
    </div>

    <div class="activity-container">
      <WorkspaceActivity
        :history="workspaceHistory"
        :loading="loading"
        @select-run="(run: AutomationRun) => emit('select-run', run)"
      />
    </div>

    <div class="metrics-container">
      <SystemMetricsPanel :metrics="metrics" />
    </div>
  </div>
</template>

<style scoped lang="postcss">
.right-pane {
  @apply w-full lg:w-72 flex flex-col gap-4 overflow-y-auto relative shrink-0 min-h-0;
}

.pulse-container {
  @apply sticky top-0 z-20 bg-gray-900 pb-4 pt-1;
  @apply flex justify-center;
}

.action-card {
  @apply bg-gray-800 rounded-lg p-3 shrink-0 border border-white/5 shadow-lg flex flex-col gap-2;
}

.action-title {
  @apply font-bold text-[10px] text-gray-500 uppercase tracking-widest;
}

.action-helper {
  @apply text-[10px] text-gray-500 mt-3 text-center italic;
}

.activity-container {
  @apply flex-1 min-h-0 bg-gray-800 rounded-lg overflow-hidden border border-white/5 shadow-lg flex flex-col;
}

.metrics-container {
  @apply shrink-0;
}
</style>
