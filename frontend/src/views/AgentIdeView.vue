<script setup lang="ts">
import { onMounted, onUnmounted, ref, computed } from "vue";
import { useDispatcher } from "../composables/useDispatcher";
import { useModels } from "../composables/useModels";
import type { Automation } from "../types/dispatcher";
import { DispatcherService } from "../services/dispatcherService";

import WorkspaceExplorer from "../components/AgentIde/WorkspaceExplorer.vue";
import AutomationForm from "../components/AgentIde/AutomationForm.vue";
import AutomationsPanel from "../components/AgentIde/AutomationsPanel.vue";
import WorkspaceActivity from "../components/AgentIde/WorkspaceActivity.vue";
import FileEditor from "../components/AgentIde/FileEditor.vue";
import SystemMetricsPanel from "../components/AgentIde/SystemMetricsPanel.vue";
import MarkdownViewer from "../components/common/MarkdownViewer.vue";

import type { AutomationRun } from "../types/dispatcher";

const { state: adminState, refresh: refreshModels } = useModels();

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
  return automations.value.find((a: any) => a.id === selectedAutomationId.value) || null;
});
const editAutomation = ref<Automation | null>(null);
const selectedWorkspace = ref<string | null>(null);
const selectedFile = ref<{ workspace: string; filename: string } | null>(null);
const fileContent = ref<string>("");
const loadingFile = ref(false);
const savingFile = ref(false);

const triggering = ref(false);
const lastTriggerResult = ref<string | null>(null);
const showHistory = ref(false);
const workspaceHistory = ref<AutomationRun[]>([]);

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
  
  // Start background polling for history to keep the "Pulse" alive
  historyInterval = setInterval(refreshHistory, 10000);
});

onUnmounted(() => {
  if (historyInterval) clearInterval(historyInterval);
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
  selectedRun.value = run;
  selectedAutomationId.value = null;
  selectedFile.value = null;
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
};

const handleSelectWorkspace = async (wsId: string) => {
  selectedWorkspace.value = selectedWorkspace.value === wsId ? null : wsId;
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
    alert("Error saving file");
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
    alert("Error creating file");
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
  }
};

const handleCreateAutomation = async (workspace: string, data: any) => {
  try {
    await createAutomation(workspace, data);
  } catch (err) {
    console.error("Error creating automation", err);
    alert("Error creating automation");
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
    alert("Error updating automation");
  }
};
</script>

<template>
  <div class="h-[calc(100vh-8rem)] flex gap-4">
    <!-- Left Pane: Sidebar -->
    <div class="w-80 flex flex-col bg-gray-800 rounded-lg overflow-hidden relative">
      <!-- Global Error Banner -->
      <div
        v-if="error"
        class="absolute top-0 left-0 right-0 z-50 p-3 bg-red-900/90 backdrop-blur-sm border-b border-red-800/50 flex flex-col gap-2 animate-in slide-in-from-top duration-300"
      >
        <div class="flex items-start justify-between gap-3">
          <div class="flex gap-2 text-red-200">
            <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4 shrink-0 mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
            <span class="text-[11px] leading-tight font-medium">{{ error }}</span>
          </div>
          <button
            @click="clearError"
            class="shrink-0 p-1 -m-1 text-red-400 hover:text-red-100 transition-colors rounded-full hover:bg-white/10"
            title="Dismiss error"
          >
            <svg xmlns="http://www.w3.org/2000/svg" class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
      </div>

      <div class="p-4 border-b border-gray-700 flex flex-col gap-3">
        <div class="flex bg-gray-900 rounded p-1">
          <button
            @click="leftTab = 'explorer'"
            :class="[
              'flex-1 py-1 text-xs font-medium rounded',
              leftTab === 'explorer'
                ? 'bg-gray-700 text-white'
                : 'text-gray-400 hover:text-gray-200',
            ]"
          >
            Explorer
          </button>
          <button
            @click="leftTab = 'automations'"
            :class="[
              'flex-1 py-1 text-xs font-medium rounded',
              leftTab === 'automations'
                ? 'bg-gray-700 text-white'
                : 'text-gray-400 hover:text-gray-200',
            ]"
          >
            Automations
          </button>
        </div>
      </div>

      <div class="flex-1 overflow-y-auto">
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

          <div v-if="loading" class="p-4 text-gray-500 text-sm">Loading...</div>

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
      <div class="flex-1 flex flex-col bg-gray-800 rounded-lg overflow-hidden border border-white/5 shadow-2xl">
        <!-- Default Dashboard View (Flat Timeline) -->
        <div
          v-if="!selectedAutomation && !selectedFile && !selectedRun"
          class="flex-1 flex flex-col p-8 overflow-y-auto bg-gray-900/40 relative"
        >
          <div class="max-w-4xl mx-auto w-full">
            <header class="flex flex-col gap-6 mb-12">
              <div class="flex items-center justify-between">
                <div>
                  <h1 class="text-3xl font-black text-white tracking-tighter mb-1 flex items-center gap-3">
                    System Pulse
                    <span v-if="selectedWorkspace" class="text-sm font-bold bg-blue-500/20 text-blue-400 px-3 py-1 rounded-full border border-blue-500/20 flex items-center gap-2">
                      {{ selectedWorkspace }}
                      <button @click="selectedWorkspace = null" class="hover:text-white transition-colors">✕</button>
                    </span>
                  </h1>
                  <p class="text-[10px] text-blue-400 font-bold uppercase tracking-[0.2em]">
                    {{ selectedWorkspace ? 'Filtered Operational Stream' : 'Global Real-time Operational Timeline' }}
                  </p>
                </div>
                <!-- Mini Stats / Filter -->
                <div class="flex gap-4">
                  <div class="px-3 py-1.5 bg-gray-800 rounded-lg border border-white/5 flex items-center gap-3">
                    <span class="text-[10px] text-gray-500 font-bold uppercase">Health</span>
                    <div class="flex gap-1">
                      <div v-for="i in 5" :key="i" class="w-1.5 h-3 bg-green-500/40 rounded-sm"></div>
                    </div>
                  </div>
                </div>
              </div>

              <!-- Filter Bar -->
              <div class="flex items-center gap-2 pb-4 border-b border-white/5">
                <button class="px-3 py-1 text-[10px] font-bold uppercase tracking-widest bg-blue-600 text-white rounded">All Activity</button>
                <button class="px-3 py-1 text-[10px] font-bold uppercase tracking-widest text-gray-500 hover:text-gray-300 transition-colors">Errors Only</button>
                <div class="flex-1"></div>
                <div v-if="loading" class="animate-spin h-4 w-4 border-2 border-blue-500 border-t-transparent rounded-full"></div>
              </div>
            </header>

            <div v-if="workspaceHistory.length === 0" class="flex flex-col items-center justify-center h-64 border-2 border-dashed border-gray-700/50 rounded-2xl">
              <p class="text-gray-500 italic text-sm">No operational history found. Your fleet is waiting for instructions.</p>
            </div>

            <div v-else class="relative pl-8 space-y-12">
              <!-- Chronological Rail -->
              <div class="absolute left-3 top-2 bottom-2 w-[2px] bg-gradient-to-b from-blue-500/50 via-gray-700 to-gray-800/0"></div>

              <div 
                v-for="run in [...workspaceHistory].reverse()" 
                :key="run.id"
                @click="handleSelectRun(run)"
                class="relative group cursor-pointer"
              >
                <!-- Timeline Marker -->
                <div :class="['absolute -left-[25px] top-1.5 w-4 h-4 rounded-full border-2 border-gray-900 z-10 transition-transform group-hover:scale-125', run.error ? 'bg-red-500 shadow-[0_0_10px_rgba(239,68,68,0.5)]' : 'bg-green-500 shadow-[0_0_10px_rgba(34,197,94,0.5)]']"></div>

                <div class="bg-gray-800/40 border border-white/5 rounded-2xl p-6 hover:bg-gray-800/80 hover:border-gray-500/20 transition-all duration-300 shadow-xl group-hover:translate-x-1">
                  <div class="flex items-start justify-between mb-6">
                    <div class="flex flex-col gap-1">
                      <div class="flex items-center gap-3">
                        <span class="text-xs font-black text-white tracking-tight uppercase">{{ run.automation_name }}</span>
                        <span v-if="!selectedWorkspace" class="px-2 py-0.5 bg-gray-700/50 text-[9px] text-gray-400 font-bold rounded uppercase tracking-widest border border-white/5">
                          {{ run.workspace_id || 'Global' }}
                        </span>
                      </div>
                      <span class="text-[10px] text-gray-500 font-mono">{{ new Date(run.timestamp).toLocaleString() }}</span>
                    </div>
                    <div class="flex flex-col items-end gap-1">
                      <span class="text-[10px] font-bold text-gray-400 tracking-widest uppercase">{{ run.model || 'System Default' }}</span>
                      <span class="text-[10px] text-gray-600 font-mono">{{ run.duration_ms }}ms execution</span>
                    </div>
                  </div>
                  
                  <!-- Preview Area -->
                  <div class="relative">
                    <div v-if="run.error" class="bg-red-900/10 border border-red-900/20 rounded-lg p-4 text-[11px] text-red-300 font-mono">
                      <span class="text-red-500 font-bold uppercase mr-2">[Fail]</span> {{ run.error }}
                    </div>
                    
                    <div v-if="run.output" class="bg-black/10 border border-white/5 rounded-lg p-4 text-[11px] text-gray-400 font-mono relative overflow-x-auto group-hover:bg-black/30 transition-colors">
                      <div class="line-clamp-4 leading-[1.1] whitespace-pre">
                        {{ run.output.replace(/\[\d{4}-\d{2}-\d{2}.*?\].*?\n/g, '').replace(/\*\*Output:\*\*\n\n```(text)?\n/, '').replace(/\n```$/, '') }}
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
            
            <div class="mt-16 text-center">
              <span class="text-[10px] font-bold text-gray-600 uppercase tracking-[0.3em]">End of operational ledger</span>
            </div>
          </div>
        </div>

      <!-- Historical Run View -->
      <div v-else-if="selectedRun" class="flex-1 flex flex-col h-full animate-in fade-in zoom-in-95 duration-300">
        <div class="p-6 border-b border-gray-700 bg-gray-900/10">
          <div class="flex items-center justify-between mb-4">
            <h2 class="text-xl font-bold text-gray-100 flex items-center gap-3">
              <span :class="['w-3 h-3 rounded-full', selectedRun.error ? 'bg-red-500' : 'bg-green-500']"></span>
              <span class="text-gray-500 text-sm font-normal">History /</span> {{ selectedRun.automation_name }}
            </h2>
            <div class="flex items-center gap-4">
              <span class="text-[10px] px-2 py-1 bg-gray-700/50 rounded text-gray-400 font-mono border border-white/5">{{ selectedRun.id }}</span>
              <button 
                @click="handleCloseDetails"
                class="bg-gray-700 hover:bg-gray-600 text-white p-1.5 rounded-full transition-colors flex items-center justify-center group"
                title="Close and return to dashboard"
              >
                <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
          </div>
          
          <div class="grid grid-cols-3 gap-6">
            <div class="bg-gray-800/40 p-3 rounded-lg border border-white/5">
              <span class="text-[10px] uppercase font-bold text-gray-500 block mb-1">Status</span>
              <span :class="['text-sm font-medium', selectedRun.error ? 'text-red-400' : 'text-green-400']">
                {{ selectedRun.error ? 'Failed' : 'Success' }}
              </span>
            </div>
             <div class="bg-gray-800/40 p-3 rounded-lg border border-white/5">
              <span class="text-[10px] uppercase font-bold text-gray-500 block mb-1">Duration</span>
              <span class="text-sm font-medium text-gray-300 font-mono">{{ selectedRun.duration_ms }} ms</span>
            </div>
             <div class="bg-gray-800/40 p-3 rounded-lg border border-white/5">
              <span class="text-[10px] uppercase font-bold text-gray-500 block mb-1">Model</span>
              <span class="text-sm font-medium text-gray-300">{{ selectedRun.model || 'Default' }}</span>
            </div>
          </div>
        </div>

        <div class="flex-1 p-6 overflow-y-auto bg-gray-900/20">
          <div v-if="selectedRun.error" class="mb-8">
            <h4 class="text-xs font-black text-red-500/80 uppercase tracking-widest mb-3">Error Context</h4>
            <div class="bg-red-900/10 border border-red-900/20 p-4 rounded-lg font-mono text-sm text-red-300 whitespace-pre-wrap">
              {{ selectedRun.error }}
            </div>
          </div>

          <div v-if="selectedRun.output">
            <h4 class="text-xs font-black text-blue-500/80 uppercase tracking-widest mb-3">Execution Output</h4>
            <div class="bg-gray-950/40 border border-white/5 p-6 rounded-xl shadow-2xl">
              <MarkdownViewer :content="selectedRun.output" />
            </div>
          </div>
        </div>
      </div>

      <!-- Editor View -->
      <div v-else-if="selectedFile" class="flex-1 flex flex-col h-full animate-in fade-in zoom-in-95 duration-300">
        <div class="px-6 py-4 border-b border-gray-700 bg-gray-900/10 flex items-center justify-between">
          <h2 class="text-sm font-bold text-gray-100 flex items-center gap-3 italic">
            <span class="text-blue-500">editing /</span> {{ selectedFile.filename }}
          </h2>
          <button 
            @click="handleCloseDetails"
            class="bg-gray-700 hover:bg-gray-600 text-white p-1.5 rounded-full transition-colors flex items-center justify-center group"
            title="Close editor and return to dashboard"
          >
            <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
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
      <template v-else-if="selectedAutomation">
        <div class="p-6 border-b border-gray-700 bg-gray-900/10 flex items-center justify-between">
          <div>
            <h2 class="text-xl font-bold text-gray-100 flex items-center gap-3">
              <span class="text-blue-500 text-sm font-normal italic">automation /</span> {{ selectedAutomation.name }}
            </h2>
            <p class="text-[10px] text-gray-500 font-bold uppercase tracking-widest mt-1">
              Workspace Scope: <span class="text-gray-400">{{ selectedAutomation.workspace }}</span>
            </p>
          </div>
          <button 
            @click="handleCloseDetails"
            class="bg-gray-700 hover:bg-gray-600 text-white p-1.5 rounded-full transition-colors flex items-center justify-center group"
            title="Close details and return to dashboard"
          >
            <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
        <div class="flex-1 p-8 overflow-y-auto bg-gray-900/10">
          <div class="grid grid-cols-3 gap-6 mb-12">
            <div class="bg-gray-800/40 p-4 rounded-xl border border-white/5">
              <span class="text-[10px] uppercase font-bold text-gray-500 block mb-2 tracking-widest">Trigger</span>
              <span class="text-sm font-medium text-blue-400 font-mono">{{ selectedAutomation.trigger }}</span>
            </div>
            <div class="bg-gray-800/40 p-4 rounded-xl border border-white/5">
              <span class="text-[10px] uppercase font-bold text-gray-500 block mb-2 tracking-widest">Strategy</span>
              <span class="text-sm font-medium text-gray-300">{{ selectedAutomation.strategy }}</span>
            </div>
            <div class="bg-gray-800/40 p-4 rounded-xl border border-white/5">
              <span class="text-[10px] uppercase font-bold text-gray-500 block mb-2 tracking-widest">Task File</span>
              <span class="text-sm font-medium text-gray-300 font-mono">{{ selectedAutomation.task_file }}</span>
            </div>
          </div>

          <div
            v-if="lastTriggerResult"
            :class="[
              'p-4 rounded-xl text-sm mb-8 border animate-in zoom-in-95 duration-300',
              lastTriggerResult.includes('Failed')
                ? 'bg-red-900/10 border-red-900/30 text-red-300'
                : 'bg-green-900/10 border-green-900/30 text-green-300',
            ]"
          >
            {{ lastTriggerResult }}
          </div>

          <div v-if="selectedAutomation.last_error" class="mb-6">
            <h4 class="text-xs font-bold text-red-400 uppercase tracking-wider mb-2">Last Error</h4>
            <div class="bg-red-900/20 border border-red-900/50 rounded p-3 text-red-200 text-sm font-mono whitespace-pre-wrap">
              {{ selectedAutomation.last_error }}
            </div>
          </div>

          <div v-if="selectedAutomation.last_output" class="mb-6">
            <div class="flex items-center justify-between mb-3">
              <h4 class="text-xs font-bold text-blue-400 uppercase tracking-wider">Last Execution Output</h4>
              <button 
                v-if="selectedAutomation.history && selectedAutomation.history.length > 0"
                @click="showHistory = !showHistory"
                class="text-[10px] text-gray-500 hover:text-blue-400 uppercase tracking-widest font-bold transition-colors"
              >
                {{ showHistory ? 'Show Current' : 'View History' }}
              </button>
            </div>
            
            <div v-if="!showHistory" class="bg-gray-900/50 border border-gray-700/50 rounded-lg p-5 shadow-inner animate-in fade-in duration-300">
              <MarkdownViewer :content="selectedAutomation.last_output" />
            </div>

            <!-- History List -->
            <div v-else class="space-y-3 animate-in slide-in-from-right duration-300">
              <div 
                v-for="run in [...(selectedAutomation.history || [])].reverse()" 
                :key="run.id"
                class="bg-gray-900/40 border border-gray-700/30 rounded p-4 hover:border-gray-500/50 transition-colors"
              >
                <div class="flex items-start justify-between mb-3">
                  <div class="flex flex-col gap-1">
                    <span class="text-[10px] font-mono text-gray-500">{{ new Date(run.timestamp).toLocaleString() }}</span>
                    <div class="flex gap-2 items-center">
                      <span :class="['text-[9px] px-1.5 py-0.5 rounded font-bold uppercase', run.error ? 'bg-red-900/40 text-red-400' : 'bg-green-900/40 text-green-400']">
                        {{ run.error ? 'Error' : 'Success' }}
                      </span>
                      <span class="text-xs text-gray-300 font-medium">{{ run.model || 'Default Model' }}</span>
                    </div>
                  </div>
                  <span class="text-[10px] text-gray-500 font-mono">{{ run.duration_ms }}ms</span>
                </div>
                
                <div v-if="run.error" class="text-xs text-red-300 font-mono bg-red-900/10 p-2 rounded mb-2 border border-red-900/20">
                  {{ run.error }}
                </div>
                
                <div v-if="run.output" class="bg-black/20 rounded p-3 border border-white/5 shadow-inner">
                  <MarkdownViewer :content="run.output" />
                </div>
              </div>
            </div>
          </div>

          <div v-if="!selectedAutomation.last_output && !selectedAutomation.last_error" class="text-gray-500 text-sm italic">
            <p>No execution history available for this automation.</p>
            <p class="text-xs mt-1">Run the automation to see the output here.</p>
          </div>
        </div>
      </template>
    </div>

    <!-- Right Pane: Monitor & Activity -->
    <div class="w-80 flex flex-col gap-4 overflow-hidden">
      <!-- Trigger Control -->
      <div class="bg-gray-800 rounded-lg p-4 shrink-0 border border-white/5 shadow-lg">
        <h3 class="font-bold text-xs text-gray-400 uppercase tracking-widest mb-4">
          Actions
        </h3>
        <button
          @click="handleTrigger"
          :disabled="!selectedAutomation || triggering"
          :class="[
            'w-full py-2.5 px-4 rounded font-bold text-xs uppercase tracking-widest transition-all duration-200 shadow-sm',
            !selectedAutomation || triggering
              ? 'bg-gray-700/50 text-gray-500 cursor-not-allowed border border-transparent'
              : 'bg-blue-600 hover:bg-blue-500 text-white border border-blue-400/30'
          ]"
        >
          {{ triggering ? "Executing..." : "Run Automation" }}
        </button>
        <p v-if="!selectedAutomation" class="text-[10px] text-gray-500 mt-3 text-center italic">
          Select an automation to enable execution
        </p>
      </div>

      <!-- Workspace Activity (The Ledger) -->
      <div class="flex-1 min-h-0 bg-gray-800 rounded-lg overflow-hidden border border-white/5 shadow-lg flex flex-col">
        <WorkspaceActivity
          :history="workspaceHistory"
          :loading="loading"
          @select-run="handleSelectRun"
        />
      </div>

      <!-- System Metrics -->
      <div class="shrink-0">
        <SystemMetricsPanel :metrics="metrics" />
      </div>
    </div>
  </div>
</template>
