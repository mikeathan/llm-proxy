<script setup lang="ts">
import { onMounted, onUnmounted, ref, computed, nextTick, watch } from "vue";
import { useDispatcher } from "../../composables/automation/useDispatcher";
import { useModels } from "../../composables/models/useModels";
import { useViewManager } from "../../composables/ui/useViewManager";
import { useFileEditor } from "../../composables/editor/useFileEditor";
import { useAutomationRunner } from "../../composables/automation/useAutomationRunner";
import { useWorkspaceHistory } from "../../composables/automation/useWorkspaceHistory";
import { useResponsiveLayout } from "../../composables/ui/useResponsiveLayout";
import type { Automation, AutomationRun } from "../../types/dispatcher";
import type { MemoryEntry } from "../../types/memory";
import { DispatcherService } from "../../services/automation/dispatcherService";

import WorkspaceExplorer from "./workspace/WorkspaceExplorer.vue";
import AutomationForm from "./automation/AutomationForm.vue";
import AutomationsPanel from "./automation/AutomationsPanel.vue";
import SystemPulseDashboard from "./system/SystemPulseDashboard.vue";
import HistoricalRunDetails from "./automation/HistoricalRunDetails.vue";
import AutomationDetails from "./automation/AutomationDetails.vue";
import RecordingsPanel from "./recordings/RecordingsPanel.vue";
import MemoryPanel from "./memory/MemoryPanel.vue";
import MemoryDetail from "./memory/MemoryDetail.vue";
import AssistantChat from "./assistant/AssistantChat.vue";
import WorkspaceSettings from "./workspace/WorkspaceSettings.vue"
import FileEditor from "./workspace/FileEditor.vue";
import TemplateLibrary from "./system/TemplateLibrary.vue";
import { useToast } from "../../composables/useToast";
import { useTemplates } from "../../composables/assistant/useTemplates";
import { useAssistant } from "../../composables/assistant/useAssistant";
import { useRunningActivity } from "../../composables/assistant/useRunningActivity";
import { useMetrics } from "../../composables/system/useMetrics";
import MobileTabBar from "./common/MobileTabBar.vue";
import SidebarNavTabs from "./common/SidebarNavTabs.vue";
import RightPane from "./common/RightPane.vue";
import Icon from "../icons/Icon.vue";

const {
  automations,
  metrics,
  workspaces,
  workspaceFiles,
  loading,
  fetchAutomations,
  fetchMetrics,
  triggerAutomation,
  fetchWorkspaces,
  fetchWorkspaceFiles,
  fetchWorkspaceState,
  fetchGlobalActivity,
  createWorkspace,
  deleteWorkspaceFile,
  deleteWorkspace,
  createAutomation,
  updateAutomation,
  deleteAutomation,
  stopAutomation,
  deleteRun,
  deleteAutomationRuns,
} = useDispatcher();

const { state: adminState, refresh: refreshModels } = useModels();
const { metrics: systemMetrics } = useMetrics();
const toast = useToast();

const {
  runningSessions,
  reconcileRunning,
  reconcileRunningConversation,
  loadSession,
  currentSessionId,
} = useAssistant()

const selectedWorkspace = ref<string | null>(null);
const selectedMemory = ref<MemoryEntry | null>(null);
const sidebarRef = ref<HTMLElement | null>(null);
const editAutomation = ref<Automation | null>(null);
const settingsWorkspaceId = ref<string | null>(null);
const workspaceExternalAccess = ref<Record<string, boolean>>({});
const recordingsEnabled = ref(false);

// Authoritative per-workspace "running" source. Drives the chat-menu glow and
// heals sticky local running flags when the backend reports nothing running.
const { assistantRunning, assistantConversationId } = useRunningActivity(selectedWorkspace)
watch(assistantRunning, (running) => reconcileRunning(running))
// When the backend reports which conversation is running, mark exactly that
// history row as running so the indicator survives a page refresh.
watch(assistantConversationId, (id) => reconcileRunningConversation(id))

const { isMobile } = useResponsiveLayout();
const { workspaceHistory, refreshHistory } = useWorkspaceHistory();

const {
  selectedAutomationId,
  selectedRun,
  triggering,
  lastTriggerResult,
  selectedAutomation,
  anyRunningInSelectedWorkspace,
  selectAutomation: handleSelectAutomation,
  selectRun: handleSelectRun,
  handleTrigger,
  handleReplayRecording,
  handleStopRecording,
  handleShowAutomation,
  handleStop,
  clearSelection,
} = useAutomationRunner(
  automations,
  triggerAutomation,
  stopAutomation,
  () => refreshHistory(selectedWorkspace.value, fetchWorkspaceState, fetchGlobalActivity),
  fetchAutomations,
);

// The delete composables already surface errors via a banner, so these handlers
// treat a thrown error as "action did not happen": no refresh and no view close.
async function handleDeleteRun(run: AutomationRun) {
  try {
    await deleteRun(run);
  } catch {
    return;
  }
  refreshHistory(selectedWorkspace.value, fetchWorkspaceState, fetchGlobalActivity);
}

async function handleClearAutomationRuns(auto: { workspace: string; name: string }) {
  if (!auto.workspace || !auto.name) return;
  try {
    await deleteAutomationRuns(auto.workspace, auto.name);
  } catch {
    return;
  }
  refreshHistory(selectedWorkspace.value, fetchWorkspaceState, fetchGlobalActivity);
}

async function handleDeleteRunFromHistory(run: AutomationRun) {
  try {
    await deleteRun(run);
  } catch {
    return;
  }
  handleCloseDetails();
  refreshHistory(selectedWorkspace.value, fetchWorkspaceState, fetchGlobalActivity);
}

async function handleClearAutomationRunsFromHistory(auto: { workspace: string; name: string }) {
  if (!auto.workspace || !auto.name) return;
  try {
    await deleteAutomationRuns(auto.workspace, auto.name);
  } catch {
    return;
  }
  handleCloseDetails();
  refreshHistory(selectedWorkspace.value, fetchWorkspaceState, fetchGlobalActivity);
}

const {
  selectedFile,
  fileContent,
  loadingFile,
  savingFile,
  handleOpenFile,
  handleSaveFile,
  handleCreateFile,
  closeFile,
} = useFileEditor(toast);

const {
  leftTab,
  mobilePanel,
  workspaceMiddleTab,
  activeMainView,
  memoryActive,
  canOpenAssistant,
  toggleAssistant,
  closeViewDetails,
} = useViewManager({
  selectedWorkspace,
  selectedFile,
  selectedRun,
  selectedMemory,
  settingsWorkspaceId,
  selectedAutomation,
  isMobile,
});

async function openAssistantSession(sessionId: string) {
  if (!selectedWorkspace.value) return
  currentSessionId.value = sessionId
  await loadSession(selectedWorkspace.value, sessionId)
  workspaceMiddleTab.value = 'chat'
}

const groupedByWorkspace = computed(() => {
  const groups: Record<string, Automation[]> = {};
  for (const auto of automations.value) {
    if (!groups[auto.workspace]) {
      groups[auto.workspace] = [];
    }
    groups[auto.workspace]!.push(auto);
  }
  return groups;
});

watch(workspaces, (ws) => {
  if (!selectedWorkspace.value && ws && ws.length > 0 && ws[0]) {
    selectedWorkspace.value = ws[0].id;
  }
}, { immediate: true });

const refreshExternalAccess = async () => {
  try {
    const configs = await DispatcherService.getAllWorkspaceConfigs();
    const access: Record<string, boolean> = {};
    for (const [wsId, cfg] of Object.entries(configs)) {
      const paths = (cfg as any)?.guardrails?.terminal?.allowed_external_paths;
      if (Array.isArray(paths) && paths.length > 0) {
        access[wsId] = true;
      }
    }
    workspaceExternalAccess.value = access;
  } catch (err) {
    console.error("Failed to refresh external access flags", err);
  }
};

let historyInterval: any = null;

onMounted(() => {
  fetchAutomations();
  fetchMetrics();
  fetchWorkspaces();
  refreshExternalAccess();
  refreshModels();
  refreshHistory(selectedWorkspace.value, fetchWorkspaceState, fetchGlobalActivity);

  DispatcherService.getRecordingStatus()
    .then((status) => {
      recordingsEnabled.value = status.enabled;
    })
    .catch((err) => {
      console.error("Failed to check recording status", err);
    });

  historyInterval = setInterval(() => {
    refreshHistory(selectedWorkspace.value, fetchWorkspaceState, fetchGlobalActivity);
    fetchAutomations(true);
    fetchMetrics();
  }, 10000);
});

onUnmounted(() => {
  if (historyInterval) clearInterval(historyInterval);
});

const handleEditAutomation = (auto: Automation) => {
  editAutomation.value = auto;
  leftTab.value = "automations";
  nextTick(() => {
    sidebarRef.value?.scrollTo(0, 0);
  });
};

const handleCancelEdit = () => {
  editAutomation.value = null;
};

const handleDeleteAutomation = async (auto: Automation) => {
  try {
    await deleteAutomation(auto.workspace, auto.name);
    if (selectedAutomationId.value === auto.id) {
      clearSelection();
    }
    if (editAutomation.value?.id === auto.id) {
      editAutomation.value = null;
    }
  } catch (err) {
    // Error is handled by compositor
  }
};

const handleCloseDetails = () => {
  clearSelection();
  closeFile();
  settingsWorkspaceId.value = null;
  closeViewDetails();
};

const handleSelectWorkspace = async (wsId: string) => {
  selectedWorkspace.value = selectedWorkspace.value === wsId ? null : wsId;

  clearSelection();
  closeFile();
  settingsWorkspaceId.value = null;
  workspaceMiddleTab.value = "pulse";

  if (!selectedWorkspace.value && leftTab.value === "memory") {
    leftTab.value = "explorer";
  }

  if (selectedWorkspace.value) {
    await fetchWorkspaceFiles(wsId);
  }
  await refreshHistory(selectedWorkspace.value, fetchWorkspaceState, fetchGlobalActivity);
};

const handleManageGuardrails = (wsId: string) => {
  settingsWorkspaceId.value = wsId;
  selectedWorkspace.value = wsId;
  clearSelection();
  closeFile();
};

const handleCreateWorkspace = async (name: string) => {
  await createWorkspace(name);
};

const handleDeleteWorkspace = async (wsId: string) => {
  await deleteWorkspace(wsId);
  await fetchWorkspaces();
};

const handleDeleteFile = async (wsId: string, file: string) => {
  await deleteWorkspaceFile(wsId, file);
  await fetchWorkspaceFiles(wsId);
};

const handleCreateAutomation = async (workspace: string, data: any) => {
  try {
    await createAutomation(workspace, data);
  } catch (err) {
    console.error("Error creating automation", err);
    toast.error("Error creating automation: " + err);
  }
};

const handleUpdateAutomation = async (
  workspace: string,
  oldName: string,
  data: any,
) => {
  try {
    await updateAutomation(workspace, oldName, data);
    editAutomation.value = null;
  } catch (err) {
    console.error("Error updating automation", err);
    toast.error("Error updating automation: " + err);
  }
};

const { showTemplates, handleInjectTemplate } = useTemplates(
  selectedWorkspace,
  selectedFile,
  fileContent,
  fetchWorkspaceFiles,
  handleOpenFile,
);
</script>

<template>
  <div class="ide-shell">
    <MobileTabBar
      :modelValue="mobilePanel"
      :workspaceMiddleTab="workspaceMiddleTab"
      :canOpenAssistant="canOpenAssistant"
      :chat-running="assistantRunning"
      :running-assistant-count="runningSessions.length"
      @update:modelValue="mobilePanel = $event"
      @toggle-chat="toggleAssistant"
    />

    <!-- Left Pane: Sidebar -->
    <div v-show="!isMobile || mobilePanel === 'explorer'" class="sidebar">
      <SidebarNavTabs
        :modelValue="leftTab"
        :recordingsEnabled="recordingsEnabled"
        @update:modelValue="leftTab = $event"
      />

      <div ref="sidebarRef" class="sidebar-content">
        <!-- Explorer Tab -->
        <WorkspaceExplorer
          v-if="leftTab === 'explorer'"
          :workspaces="workspaces"
          :workspaceFiles="workspaceFiles"
          :selectedWorkspace="selectedWorkspace"
          :selectedFile="selectedFile"
          :loading="loading"
          :workspaceExternalAccess="workspaceExternalAccess"
          :chat-active="workspaceMiddleTab === 'chat'"
          :chat-running="assistantRunning"
          :running-assistant-count="runningSessions.length"
          :memory-active="memoryActive"
          @select-workspace="handleSelectWorkspace"
          @create-workspace="handleCreateWorkspace"
          @delete-workspace="handleDeleteWorkspace"
          @open-file="handleOpenFile"
          @create-file="handleCreateFile"
          @delete-file="handleDeleteFile"
          @manage-guardrails="handleManageGuardrails"
          @open-memory="leftTab = 'memory'"
          @open-playbooks="showTemplates = true"
          @open-chat="toggleAssistant"
        />

        <!-- Automations Tab -->
        <div v-else-if="leftTab === 'automations'">
          <AutomationForm
            :workspaces="workspaces"
            :workspaceFiles="workspaceFiles"
            :hasAutomations="automations.length > 0"
            :editAutomation="editAutomation"
            @create-automation="handleCreateAutomation"
            @update-automation="handleUpdateAutomation"
            @cancel-edit="handleCancelEdit"
            @fetch-files="fetchWorkspaceFiles"
          />

          <div
            v-if="loading && automations.length === 0"
            class="loading-message"
          >
            Loading automations...
          </div>

          <AutomationsPanel
            :groupedAutomations="groupedByWorkspace"
            :selectedAutomationId="selectedAutomation?.id"
            @select-automation="handleSelectAutomation"
            @edit-automation="handleEditAutomation"
            @delete-automation="handleDeleteAutomation"
          />
        </div>

        <!-- Recordings Tab -->
        <RecordingsPanel
          v-else-if="leftTab === 'recordings'"
          :automations="automations"
          :workspaces="Object.keys(groupedByWorkspace)"
          @replay-recording="handleReplayRecording"
          @stop-automation="handleStopRecording"
          @show-automation="handleShowAutomation"
        />

        <!-- Memory Tab -->
        <MemoryPanel
          v-else-if="leftTab === 'memory'"
          :workspace-id="selectedWorkspace"
          @select-memory="(e: MemoryEntry) => selectedMemory = e"
        />
      </div>
    </div>

    <!-- Middle Pane: Details / Editor / Dashboard -->
    <div v-show="!isMobile || mobilePanel === 'workspace'" class="main-pane">
      <!-- Default Dashboard View -->
      <SystemPulseDashboard
        v-if="activeMainView === 'dashboard'"
        :selected-workspace="selectedWorkspace"
        :loading="loading"
        :workspace-history="workspaceHistory"
        @select-run="handleSelectRun"
        @clear-workspace="selectedWorkspace = null"
      />

      <!-- Historical Run View -->
      <HistoricalRunDetails
        v-else-if="activeMainView === 'history'"
        :run="selectedRun!"
        @close="handleCloseDetails"
        @delete-run="handleDeleteRunFromHistory"
        @delete-automation-runs="handleClearAutomationRunsFromHistory"
      />

      <!-- Memory Detail View -->
      <MemoryDetail
        v-else-if="activeMainView === 'memory-detail' && selectedMemory && selectedWorkspace"
        :entry="selectedMemory"
        :workspace-id="selectedWorkspace"
        @close="selectedMemory = null"
        @updated="selectedMemory = null"
      />

      <!-- Workspace Settings View -->
      <WorkspaceSettings
        v-else-if="activeMainView === 'workspace-settings'"
        :workspaceId="settingsWorkspaceId!"
        :globalGuardrails="adminState!.config.guardrails"
        @close="handleCloseDetails"
      />

      <!-- Editor View -->
      <div v-else-if="activeMainView === 'editor'" class="editor-shell">
        <div class="editor-header">
          <h2 class="editor-title">
            <span class="title-prefix">editing /</span>
            {{ selectedFile?.filename }}
          </h2>
          <button
            @click="handleCloseDetails"
            class="btn-icon-round group"
            title="Close editor and return to dashboard"
          >
            <Icon name="close" size="sm" />
          </button>
        </div>
        <FileEditor
          :file="selectedFile!"
          :content="fileContent"
          :loading="loadingFile"
          :saving="savingFile"
          @update:content="fileContent = $event"
          @save="handleSaveFile"
        />
      </div>

      <!-- Automation Details View -->
      <AutomationDetails
        v-else-if="activeMainView === 'automation'"
        :key="`${selectedAutomation!.id}-${selectedAutomation!.is_running || triggering}`"
        :automation="selectedAutomation!"
        :last-trigger-result="lastTriggerResult"
        :is-executing="triggering || (selectedAutomation?.is_running ?? false)"
        :selectedRun="selectedRun"
        @close="handleCloseDetails"
        @delete-run="handleDeleteRun"
        @delete-automation-runs="handleClearAutomationRuns"
      />

      <!-- Assistant View (full main pane) -->
      <AssistantChat
        v-else-if="activeMainView === 'assistant' && selectedWorkspace"
        :workspaceId="selectedWorkspace"
        @close="workspaceMiddleTab = 'pulse'"
      />
    </div>

    <RightPane
      v-show="!isMobile || mobilePanel === 'monitor'"
      :systemMetrics="systemMetrics"
      :activeModel="adminState?.active ?? null"
      :selectedAutomation="selectedAutomation"
      :anyRunningInSelectedWorkspace="anyRunningInSelectedWorkspace"
      :triggering="triggering"
      :workspaceHistory="workspaceHistory"
      :assistantSessions="runningSessions"
      :loading="loading"
      :metrics="metrics"
      @trigger="handleTrigger"
      @stop="handleStop"
      @select-run="handleSelectRun"
      @select-assistant-session="openAssistantSession"
      @delete-run="handleDeleteRun"
    />

    <!-- Modals & Overlays -->
    <TemplateLibrary
      :show="showTemplates"
      @close="showTemplates = false"
      @inject="handleInjectTemplate"
    />


  </div>
</template>

<style scoped lang="postcss">
.ide-shell {
  @apply h-[calc(100vh-10rem)] flex flex-col lg:flex-row lg:h-[calc(100vh-8rem)] gap-4;
}

.sidebar {
  @apply w-full lg:w-72 flex flex-col bg-gray-800 rounded-lg overflow-hidden relative shadow-lg shrink-0 min-h-0;
}

.sidebar-content {
  @apply flex-1 overflow-y-auto;
}

.loading-message {
  @apply p-4 text-gray-500 text-sm;
}

/* ── Main Pane ── */
.main-pane {
  @apply flex-1 flex flex-col bg-gray-800 rounded-lg overflow-hidden border border-white/5 shadow-2xl min-h-0;
}

.editor-shell {
  @apply flex-1 flex flex-col animate-in fade-in zoom-in-95 duration-300;
}

.editor-header {
  @apply px-6 py-4 border-b border-gray-700 bg-gray-900/10 flex items-center justify-between;
}

.editor-title {
  @apply text-sm font-bold text-gray-100 flex items-center gap-3 italic;
}

.title-prefix {
  @apply text-blue-500;
}

.btn-icon-round {
  @apply bg-gray-700 hover:bg-gray-600 text-white p-1.5 rounded-full transition-colors 
         flex items-center justify-center;
}




</style>
