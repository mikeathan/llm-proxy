<script setup lang="ts">
import { onMounted, onUnmounted, ref, computed, nextTick, watch } from "vue";
import { useDispatcher } from "../../composables/useDispatcher";
import { useModels } from "../../composables/useModels";
import type { Automation, RecordingMeta } from "../../types/dispatcher";
import type { MemoryEntry } from "../../types/memory";
import { DispatcherService } from "../../services/dispatcherService";

import WorkspaceExplorer from "./workspace/WorkspaceExplorer.vue";
import AutomationForm from "./automation/AutomationForm.vue";
import AutomationsPanel from "./automation/AutomationsPanel.vue";
import WorkspaceActivity from "./workspace/WorkspaceActivity.vue";
import FileEditor from "./workspace/FileEditor.vue";
import SystemMetricsPanel from "./system/SystemMetricsPanel.vue";
import SystemPulseDashboard from "./system/SystemPulseDashboard.vue";
import HistoricalRunDetails from "./automation/HistoricalRunDetails.vue";
import AutomationDetails from "./automation/AutomationDetails.vue";
import RecordingsPanel from "./recordings/RecordingsPanel.vue";
import MemoryPanel from "./memory/MemoryPanel.vue";
import MemoryDetail from "./memory/MemoryDetail.vue";
import AssistantChat from "./assistant/AssistantChat.vue";
import WorkspaceSettings from "./workspace/WorkspaceSettings.vue";
import TemplateLibrary from "./system/TemplateLibrary.vue";
import type { AutomationRun } from "../../types/dispatcher";
import { useToast } from "../../composables/useToast";
import { useTemplates } from "../../composables/useTemplates";
import { useMetrics } from "../../composables/useMetrics";
import BaseButton from "../common/BaseButton.vue";
import MetricsPulse from "../common/MetricsPulse.vue";
import Icon from "../icons/Icon.vue";

/* ── Composables & Services ── */
const { state: adminState, refresh: refreshModels } = useModels();
const { metrics: systemMetrics } = useMetrics();
const toast = useToast();

const {
  automations,
  metrics,
  workspaces,
  workspaceFiles,
  loading,
  error,
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
  clearError,
} = useDispatcher();

/* ── UI & Selection State ── */
const leftTab = ref<"explorer" | "automations" | "recordings" | "memory" | "activity">("explorer");
const recordingsEnabled = ref(false);
const selectedAutomationId = ref<string | null>(null);
const selectedRun = ref<AutomationRun | null>(null);
const selectedWorkspace = ref<string | null>(null);
const selectedMemory = ref<MemoryEntry | null>(null);
const selectedFile = ref<{ workspace: string; filename: string } | null>(null);
const sidebarRef = ref<HTMLElement | null>(null);
const editAutomation = ref<Automation | null>(null);

/* ── Content & Loading State ── */
const fileContent = ref<string>("");
const loadingFile = ref(false);
const savingFile = ref(false);
const triggering = ref(false);
const lastTriggerResult = ref<string | null>(null);

/* ── View & Layout State ── */
const workspaceHistory = ref<AutomationRun[]>([]);
const workspaceMiddleTab = ref<"pulse" | "chat">("pulse");
const settingsWorkspaceId = ref<string | null>(null);
const workspaceExternalAccess = ref<Record<string, boolean>>({});
const mobilePanel = ref<"explorer" | "workspace" | "monitor">("workspace");
const isMobile = ref(false);


/* ── Computed Properties ── */
const models = computed(() => adminState.value?.models || []);
const providers = computed(() => adminState.value?.config.providers || {});

const selectedAutomation = computed(() => {
  if (!selectedAutomationId.value) return null;
  return (
    automations.value.find((a: any) => a.id === selectedAutomationId.value) ||
    null
  );
});

const memoryActive = computed(() => leftTab.value === "memory");

const activeMainView = computed(() => {
  if (settingsWorkspaceId.value) return "workspace-settings";
  if (selectedRun.value) return "history";
  if (selectedMemory.value && selectedWorkspace.value) return "memory-detail";
  if (selectedFile.value) return "editor";
  if (selectedAutomation.value) return "automation";
  if (selectedWorkspace.value && workspaceMiddleTab.value === "chat")
    return "assistant";
  return "dashboard";
});

const canOpenAssistant = computed(
  () => !!selectedWorkspace.value
);

function toggleAssistant() {
  workspaceMiddleTab.value = workspaceMiddleTab.value === "chat" ? "pulse" : "chat";
  if (isMobile.value) {
    mobilePanel.value = "workspace";
  }
}

/* ── Methods & Handlers ── */
const updateLayout = () => {
  isMobile.value = window.innerWidth < 1024;
};

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

const refreshHistory = async () => {
  try {
    if (selectedWorkspace.value) {
      const state = await fetchWorkspaceState(selectedWorkspace.value);
      workspaceHistory.value = state.history || [];
    } else {
      const global = await fetchGlobalActivity();
      workspaceHistory.value = global || [];
    }
  } catch (err) {
    console.error("Failed to fetch history", err);
  }
};

let historyInterval: any = null;

onMounted(() => {
  fetchAutomations();
  fetchMetrics();
  fetchWorkspaces();
  refreshExternalAccess();
  refreshModels();
  refreshHistory();

  DispatcherService.getRecordingStatus()
    .then((status) => {
      recordingsEnabled.value = status.enabled;
    })
    .catch((err) => {
      console.error("Failed to check recording status", err);
    });

  updateLayout();
  window.addEventListener("resize", updateLayout);
  // Start background polling to keep the Pulse and Running States alive
  historyInterval = setInterval(() => {
    refreshHistory();
    fetchAutomations(true);
    fetchMetrics();
  }, 10000);
});

onUnmounted(() => {
  if (historyInterval) clearInterval(historyInterval);
  window.removeEventListener("resize", updateLayout);
});

// Auto-select the first workspace when none is selected and workspaces load.
// Ensures the assistant is usable in mobile view without requiring the user
// to manually open the workspace explorer first.
watch(workspaces, (ws) => {
  if (!selectedWorkspace.value && ws && ws.length > 0 && ws[0]) {
    selectedWorkspace.value = ws[0].id;
  }
}, { immediate: true });

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
const anyRunningInSelectedWorkspace = computed(() => {
  if (!selectedAutomation.value) return false;
  const workspace = selectedAutomation.value.workspace;
  return automations.value.some((a) => a.workspace === workspace && a.is_running);
});

const handleSelectAutomation = (auto: Automation) => {
  selectedAutomationId.value = auto.id;
  selectedFile.value = null;
  selectedRun.value = null;
  lastTriggerResult.value = null;
};

const handleSelectRun = (run: AutomationRun) => {
  // Find the automation this run belongs to
  const auto = automations.value.find(
    (a) => a.name === run.automation_name && a.workspace === run.workspace_id,
  );
  if (auto) {
    selectedAutomationId.value = auto.id;
    selectedRun.value = run;
    selectedFile.value = null;
    lastTriggerResult.value = null;
    workspaceMiddleTab.value = "pulse";
  } else {
    // Fallback to the latest single run view if automation record is missing
    selectedRun.value = run;
    selectedAutomationId.value = null;
    selectedFile.value = null;
  }
};

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
      selectedAutomationId.value = null;
    }
    if (editAutomation.value?.id === auto.id) {
      editAutomation.value = null;
    }
  } catch (err) {
    // Error is handled by compositor
  }
};

const handleCloseDetails = () => {
  selectedRun.value = null;
  selectedFile.value = null;
  selectedAutomationId.value = null;
  settingsWorkspaceId.value = null;
  fileContent.value = "";
  workspaceMiddleTab.value = "pulse";
};

const handleSelectWorkspace = async (wsId: string) => {
  selectedWorkspace.value = selectedWorkspace.value === wsId ? null : wsId;

  // Clear any active views when switching workspace context
  selectedAutomationId.value = null;
  selectedRun.value = null;
  selectedFile.value = null;
  settingsWorkspaceId.value = null;
  workspaceMiddleTab.value = "pulse";

  if (!selectedWorkspace.value && leftTab.value === "memory") {
    leftTab.value = "explorer";
  }

  if (selectedWorkspace.value) {
    await fetchWorkspaceFiles(wsId);
  }
  await refreshHistory();
};

const handleManageGuardrails = (wsId: string) => {
  settingsWorkspaceId.value = wsId;
  selectedWorkspace.value = wsId;
  selectedAutomationId.value = null;
  selectedRun.value = null;
  selectedFile.value = null;
};

const handleOpenFile = async (workspace: string, filename: string) => {
  selectedFile.value = { workspace, filename };
  selectedAutomationId.value = null;
  selectedRun.value = null;
  loadingFile.value = true;
  fileContent.value = "";
  try {
    fileContent.value = await DispatcherService.readWorkspaceFile(
      workspace,
      filename,
    );
  } catch (err) {
    console.error("Error loading file", err);
    fileContent.value = "Error loading file content.";
  } finally {
    loadingFile.value = false;
  }
};

const handleSaveFile = async () => {
  if (!selectedFile.value) return;
  savingFile.value = true;
  try {
    await DispatcherService.writeWorkspaceFile(
      selectedFile.value.workspace,
      selectedFile.value.filename,
      fileContent.value,
    );
    if (selectedFile.value.filename === "config.yaml") {
      setTimeout(fetchAutomations, 500);
    }
  } catch (err) {
    console.error("Error saving file", err);
    toast.error("Error saving file: " + err);
  } finally {
    savingFile.value = false;
  }
};

const handleCreateWorkspace = async (name: string) => {
  await createWorkspace(name);
};

const handleCreateFile = async (workspace: string, filename: string) => {
  try {
    await DispatcherService.writeWorkspaceFile(workspace, filename, "");
    await fetchWorkspaceFiles(workspace);
  } catch (err) {
    console.error("Error creating file", err);
    toast.error("Error creating file: " + err);
  }
};

const handleDeleteWorkspace = async (wsId: string) => {
  await deleteWorkspace(wsId);
  await fetchWorkspaces();
};

const handleDeleteFile = async (wsId: string, file: string) => {
  await deleteWorkspaceFile(wsId, file);
  await fetchWorkspaceFiles(wsId);
};

const handleTrigger = async () => {
  if (!selectedAutomation.value) return;
  triggering.value = true;
  lastTriggerResult.value = `Running ${selectedAutomation.value.name}...`;
  try {
    await triggerAutomation(
      selectedAutomation.value.workspace,
      selectedAutomation.value.name,
    );
    lastTriggerResult.value = `Triggered ${selectedAutomation.value.name} successfully`;
    await refreshHistory();
  } catch {
    lastTriggerResult.value = `Failed to trigger ${selectedAutomation.value.name}`;
  } finally {
    triggering.value = false;
    await fetchAutomations();
    await refreshHistory();
  }
};

const handleReplayRecording = async (auto: Automation, recording: RecordingMeta) => {
  selectedAutomationId.value = auto.id;
  triggering.value = true;
  lastTriggerResult.value = `Replaying recording for ${auto.name}...`;
  try {
    await triggerAutomation(auto.workspace, auto.name, recording.id);
    lastTriggerResult.value = `Replayed ${auto.name} from recording`;
    await refreshHistory();
  } catch {
    lastTriggerResult.value = `Failed to replay ${auto.name}`;
  } finally {
    triggering.value = false;
    await fetchAutomations();
    await refreshHistory();
  }
};

const handleStopRecording = async (workspace: string) => {
  try {
    await stopAutomation(workspace);
    lastTriggerResult.value = "Recording replay stopped";
  } catch (err) {
    console.error("Stop recording replay failed", err);
  } finally {
    await fetchAutomations();
  }
};

const handleShowAutomation = (id: string) => {
  selectedAutomationId.value = id;
};

const handleStop = async () => {
  if (!selectedAutomation.value) return;
  try {
    await stopAutomation(selectedAutomation.value.workspace);
    lastTriggerResult.value = `Stopped ${selectedAutomation.value.name}`;
  } catch (err) {
    console.error("Stop failed", err);
  } finally {
    await fetchAutomations();
  }
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
    <!-- Mobile Tab Bar -->
    <div class="mobile-tabs">
      <button
        @click="mobilePanel = 'explorer'; workspaceMiddleTab = 'pulse'"
        :class="[
          'mobile-tab',
          mobilePanel === 'explorer' ? 'mobile-tab--active' : '',
        ]"
      >
        Explorer
      </button>
      <button
        @click="mobilePanel = 'workspace'; workspaceMiddleTab = 'pulse'"
        :class="[
          'mobile-tab',
          mobilePanel === 'workspace' && workspaceMiddleTab === 'pulse' ? 'mobile-tab--active' : '',
        ]"
      >
        Workspace
      </button>
      <button
        @click="mobilePanel = 'monitor'; workspaceMiddleTab = 'pulse'"
        :class="[
          'mobile-tab',
          mobilePanel === 'monitor' ? 'mobile-tab--active' : '',
        ]"
      >
        Monitor
      </button>
      <button
        @click="toggleAssistant"
        :disabled="!canOpenAssistant"
        :class="[
          'mobile-tab',
          workspaceMiddleTab === 'chat' && mobilePanel === 'workspace' ? 'mobile-tab--active' : '',
        ]"
        title="Open Workspace Assistant"
      >
        Chat
      </button>
    </div>

    <!-- Left Pane: Sidebar -->
    <div v-show="!isMobile || mobilePanel === 'explorer'" class="sidebar">
      <div v-if="error" class="error-banner">
        <div class="error-content">
          <div class="error-message-row">
            <svg
              xmlns="http://www.w3.org/2000/svg"
              class="h-4 w-4 shrink-0 mt-0.5"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
              />
            </svg>
            <span class="error-text">{{ error }}</span>
          </div>
          <button @click="clearError" class="btn-dismiss" title="Dismiss error">
            <Icon name="close" size="sm" />
          </button>
        </div>
      </div>

          <div class="sidebar-header">
            <div class="sidebar-tabs-icon" :class="recordingsEnabled ? 'grid-cols-3' : 'grid-cols-2'">
              <button
                @click="leftTab = 'explorer'"
                class="sidebar-tab-icon"
                :class="
                  leftTab === 'explorer'
                    ? 'sidebar-tab-icon--active'
                    : 'sidebar-tab-icon--inactive'
                "
              >
                <Icon name="lightning" size="sm" />
                <span>Explorer</span>
              </button>
              <button
                @click="leftTab = 'automations'"
                class="sidebar-tab-icon"
                :class="
                  leftTab === 'automations'
                    ? 'sidebar-tab-icon--active'
                    : 'sidebar-tab-icon--inactive'
                "
              >
                <Icon name="play" size="sm" />
                <span>Automations</span>
              </button>
              <button
                v-if="recordingsEnabled"
                @click="leftTab = 'recordings'"
                class="sidebar-tab-icon"
                :class="
                  leftTab === 'recordings'
                    ? 'sidebar-tab-icon--active'
                    : 'sidebar-tab-icon--inactive'
                "
              >
                <Icon name="refresh" size="sm" />
                <span>Recordings</span>
              </button>
            </div>
          </div>

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
            :models="models"
            :providers="providers"
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
      />

      <!-- Assistant View (full main pane) -->
      <AssistantChat
        v-else-if="activeMainView === 'assistant' && selectedWorkspace"
        :workspaceId="selectedWorkspace"
        @close="workspaceMiddleTab = 'pulse'"
      />
    </div>

    <!-- Right Pane: Monitor & Activity -->
    <div v-show="!isMobile || mobilePanel === 'monitor'" class="right-pane">
      <!-- Hardware Pulse -->
      <div class="pulse-container">
        <MetricsPulse :metrics="systemMetrics" :activeModel="adminState?.active" />
      </div>

      <!-- Trigger Control -->
      <div class="action-card">
        <h3 class="action-title">Actions</h3>
        <BaseButton
          v-if="!anyRunningInSelectedWorkspace"
          @click="handleTrigger"
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
          @click="handleStop"
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

      <!-- Workspace Activity (The Ledger) -->
      <div class="activity-container">
        <WorkspaceActivity
          :history="workspaceHistory"
          :loading="loading"
          @select-run="handleSelectRun"
        />
      </div>

      <!-- System Metrics -->
      <div class="metrics-container">
        <SystemMetricsPanel :metrics="metrics" />
      </div>
    </div>

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

/* Mobile tab bar - only shown on small screens */
.mobile-tabs {
  @apply flex lg:hidden gap-1 bg-gray-800/50 rounded-xl p-1 shrink-0 border border-white/5;
}

.mobile-tab {
  @apply flex-1 py-2 px-3 text-xs font-semibold rounded-lg transition-colors text-gray-400;
}

.mobile-tab--active {
  @apply bg-blue-600 text-white shadow-md;
}

/* ── Sidebar ── */
.sidebar {
  @apply w-full lg:w-72 flex flex-col bg-gray-800 rounded-lg overflow-hidden relative shadow-lg shrink-0 min-h-0;
}

.error-banner {
  @apply absolute top-0 left-0 right-0 z-50 p-3 bg-red-900/90 backdrop-blur-sm 
         border-b border-red-800/50 flex flex-col gap-2 animate-in slide-in-from-top duration-300;
}

.error-content {
  @apply flex items-start justify-between gap-3;
}

.error-message-row {
  @apply flex gap-2 text-red-200;
}

.error-text {
  @apply text-[11px] leading-tight font-medium;
}

.btn-dismiss {
  @apply shrink-0 p-1 -m-1 text-red-400 hover:text-red-100 transition-colors rounded-full hover:bg-white/10;
}

.sidebar-header {
  @apply p-2 px-3 border-b border-gray-700;
}

.sidebar-tabs-icon {
  @apply grid gap-1 bg-gray-900 rounded p-0.5;
}

.sidebar-tab-icon {
  @apply flex items-center justify-center gap-1.5 h-7 px-2 rounded transition-colors 
         text-[10px] font-medium text-gray-400 truncate
         disabled:opacity-30 disabled:cursor-not-allowed;
}

.sidebar-tab-icon--active {
  @apply bg-gray-700 text-white;
}

.sidebar-tab-icon--inactive {
  @apply hover:text-gray-200;
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
  @apply flex-1 flex flex-col h-full animate-in fade-in zoom-in-95 duration-300;
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

/* ── Right Pane ── */
.right-pane {
  @apply w-full lg:w-72 flex flex-col gap-4 overflow-y-auto relative shrink-0 min-h-0;
}

.pulse-container {
  @apply sticky top-0 z-20 bg-gray-900/80 backdrop-blur-md pb-4 pt-1;
  @apply flex justify-center;
}

.action-card {
  @apply bg-gray-800 rounded-lg p-3 shrink-0 border border-white/5 shadow-lg flex flex-col gap-2;
}

.action-title {
  @apply font-bold text-[10px] text-gray-500 uppercase tracking-widest;
}

.btn-action {
  @apply w-full py-2 px-4 rounded font-bold text-[10px] uppercase tracking-widest 
         transition-all duration-200 shadow-sm bg-blue-600 hover:bg-blue-500 text-white border border-blue-400/30;
}

.btn-action--disabled {
  @apply bg-gray-700/50 text-gray-500 cursor-not-allowed border-transparent shadow-none;
}

.btn-action--stop {
  @apply bg-red-600 hover:bg-red-500 border-red-400/30;
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
