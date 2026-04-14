<script setup lang="ts">
import { onMounted, onUnmounted, ref, computed } from "vue";
import { useDispatcher } from "../../composables/useDispatcher";
import { useModels } from "../../composables/useModels";
import type { Automation } from "../../types/dispatcher";
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
import AssistantChat from "./assistant/AssistantChat.vue";

import type { AutomationRun } from "../../types/dispatcher";
import { useToast } from "../../composables/useToast";

const { state: adminState, refresh: refreshModels } = useModels();
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
  clearError,
} = useDispatcher();

const models = computed(() => adminState.value?.models || []);
const providers = computed(() => adminState.value?.config.providers || {});

const leftTab = ref<"explorer" | "automations" | "activity">("explorer");
const selectedAutomationId = ref<string | null>(null);
const selectedRun = ref<AutomationRun | null>(null);

const selectedAutomation = computed(() => {
  if (!selectedAutomationId.value) return null;
  return (
    automations.value.find((a: any) => a.id === selectedAutomationId.value) ||
    null
  );
});
const editAutomation = ref<Automation | null>(null);
const selectedWorkspace = ref<string | null>(null);
const selectedFile = ref<{ workspace: string; filename: string } | null>(null);
const fileContent = ref<string>("");
const loadingFile = ref(false);
const savingFile = ref(false);

const triggering = ref(false);
const lastTriggerResult = ref<string | null>(null);
const workspaceHistory = ref<AutomationRun[]>([]);
const workspaceMiddleTab = ref<"pulse" | "chat">("pulse");


const mobilePanel = ref<"explorer" | "workspace" | "monitor">("workspace");
const isMobile = ref(false);

const updateLayout = () => {
  isMobile.value = window.innerWidth < 1024;
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
  refreshModels();
  refreshHistory();

  updateLayout();
  // Start background polling to keep the Pulse and Running States alive
  historyInterval = setInterval(() => {
    refreshHistory();
    fetchAutomations();
  }, 10000);
});

onUnmounted(() => {
  if (historyInterval) clearInterval(historyInterval);
  window.removeEventListener("resize", updateLayout);
});

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
  fileContent.value = "";
  workspaceMiddleTab.value = "pulse";
};

const handleSelectWorkspace = async (wsId: string) => {
  selectedWorkspace.value = selectedWorkspace.value === wsId ? null : wsId;
  
  // Clear any active views when switching workspace context
  selectedAutomationId.value = null;
  selectedRun.value = null;
  selectedFile.value = null;
  workspaceMiddleTab.value = "pulse";


  if (selectedWorkspace.value) {
    await fetchWorkspaceFiles(wsId);
  }
  await refreshHistory();
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
  lastTriggerResult.value = null;
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
</script>

<template>
  <div class="ide-shell">
    <!-- Mobile Tab Bar -->
    <div class="mobile-tabs">
      <button
        @click="mobilePanel = 'explorer'"
        :class="[
          'mobile-tab',
          mobilePanel === 'explorer' ? 'mobile-tab--active' : '',
        ]"
      >
        Explorer
      </button>
      <button
        @click="mobilePanel = 'workspace'"
        :class="[
          'mobile-tab',
          mobilePanel === 'workspace' ? 'mobile-tab--active' : '',
        ]"
      >
        Workspace
      </button>
      <button
        @click="mobilePanel = 'monitor'"
        :class="[
          'mobile-tab',
          mobilePanel === 'monitor' ? 'mobile-tab--active' : '',
        ]"
      >
        Monitor
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
            <svg
              xmlns="http://www.w3.org/2000/svg"
              class="h-3.5 w-3.5"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M6 18L18 6M6 6l12 12"
              />
            </svg>
          </button>
        </div>
      </div>

      <div class="sidebar-header">
        <div class="sidebar-tabs">
          <button
            @click="leftTab = 'explorer'"
            class="sidebar-tab"
            :class="
              leftTab === 'explorer'
                ? 'sidebar-tab--active'
                : 'sidebar-tab--inactive'
            "
          >
            Explorer
          </button>
          <button
            @click="leftTab = 'automations'"
            class="sidebar-tab"
            :class="
              leftTab === 'automations'
                ? 'sidebar-tab--active'
                : 'sidebar-tab--inactive'
            "
          >
            Automations
          </button>
        </div>
      </div>

      <div class="sidebar-content">
        <!-- Explorer Tab -->
        <WorkspaceExplorer
          v-if="leftTab === 'explorer'"
          :workspaces="workspaces"
          :workspaceFiles="workspaceFiles"
          :selectedWorkspace="selectedWorkspace"
          :selectedFile="selectedFile"
          :loading="loading"
          @select-workspace="handleSelectWorkspace"
          @create-workspace="handleCreateWorkspace"
          @delete-workspace="handleDeleteWorkspace"
          @open-file="handleOpenFile"
          @create-file="handleCreateFile"
          @delete-file="handleDeleteFile"
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

          <div v-if="loading" class="loading-message">Loading...</div>

          <AutomationsPanel
            :groupedAutomations="groupedByWorkspace"
            :selectedAutomationId="selectedAutomation?.id"
            @select-automation="handleSelectAutomation"
            @edit-automation="handleEditAutomation"
            @delete-automation="handleDeleteAutomation"
          />
        </div>
      </div>
    </div>

    <!-- Middle Pane: Details / Editor / Dashboard -->
    <div v-show="!isMobile || mobilePanel === 'workspace'" class="main-pane">
      <!-- Assistant View -->
      <AssistantChat
        v-if="
          !selectedAutomation &&
          !selectedFile &&
          !selectedRun &&
          selectedWorkspace &&
          workspaceMiddleTab === 'chat'
        "
        :workspaceId="selectedWorkspace"
        @close="workspaceMiddleTab = 'pulse'"
      />

      <!-- Default Dashboard View (Flat Timeline) - Now used for both Global and Workspace Pulse -->
      <SystemPulseDashboard
        v-else-if="
          !selectedAutomation &&
          !selectedFile &&
          !selectedRun &&
          (!selectedWorkspace || workspaceMiddleTab === 'pulse')
        "
        :selected-workspace="selectedWorkspace"
        :loading="loading"
        :workspace-history="workspaceHistory"
        @select-run="handleSelectRun"
        @clear-workspace="selectedWorkspace = null"
        @open-chat="workspaceMiddleTab = 'chat'"
      />


      <!-- Historical Run View -->
      <HistoricalRunDetails
        v-else-if="selectedRun"
        :run="selectedRun"
        @close="handleCloseDetails"
      />

      <!-- Editor View -->
      <div v-else-if="selectedFile" class="editor-shell">
        <div class="editor-header">
          <h2 class="editor-title">
            <span class="title-prefix">editing /</span>
            {{ selectedFile.filename }}
          </h2>
          <button
            @click="handleCloseDetails"
            class="btn-icon-round group"
            title="Close editor and return to dashboard"
          >
            <svg
              xmlns="http://www.w3.org/2000/svg"
              class="h-4 w-4"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M6 18L18 6M6 6l12 12"
              />
            </svg>
          </button>
        </div>
        <FileEditor
          :file="selectedFile"
          :content="fileContent"
          :loading="loadingFile"
          :saving="savingFile"
          @update:content="fileContent = $event"
          @save="handleSaveFile"
        />
      </div>

      <!-- Automation Details View -->
      <AutomationDetails
        v-else-if="selectedAutomation"
        :automation="selectedAutomation"
        :last-trigger-result="lastTriggerResult"
        :selectedRun="selectedRun"
        @close="handleCloseDetails"
      />
    </div>

    <!-- Right Pane: Monitor & Activity -->
    <div v-show="!isMobile || mobilePanel === 'monitor'" class="right-pane">
      <!-- Trigger Control -->
      <div class="action-card">
        <h3 class="action-title">Actions</h3>
        <button
          @click="handleTrigger"
          :disabled="!selectedAutomation || triggering"
          class="btn-action"
          :class="{ 'btn-action--disabled': !selectedAutomation || triggering }"
        >
          {{ triggering ? "Executing..." : "Run Automation" }}
        </button>
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
  @apply p-3 px-4 border-b border-gray-700 flex flex-col gap-2.5;
}

.sidebar-tabs {
  @apply flex bg-gray-900 rounded p-1;
}

.sidebar-tab {
  @apply flex-1 py-1 text-xs font-medium rounded transition-colors;
}

.sidebar-tab--active {
  @apply bg-gray-700 text-white;
}

.sidebar-tab--inactive {
  @apply text-gray-400 hover:text-gray-200;
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
