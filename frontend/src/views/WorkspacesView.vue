<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue"
import { useRouter } from "vue-router"
import { useDispatcher } from "../composables/automation/useDispatcher"
import { useModels } from "../composables/models/useModels"
import { useViewManager } from "../composables/ui/useViewManager"
import { useFileEditor } from "../composables/editor/useFileEditor"
import { useWorkspaceHistory } from "../composables/automation/useWorkspaceHistory"
import { useResponsiveLayout } from "../composables/ui/useResponsiveLayout"
import { useDestinationRoute } from "../composables/ui/useDestinationRoute"
import { useUnsavedChangesGuard } from "../composables/ui/useUnsavedChangesGuard"
import { useLastWorkspace } from "../composables/ui/useLastWorkspace"
import UnsavedTag from "../components/common/display/UnsavedTag.vue"
import CopyButton from "../components/common/display/CopyButton.vue"
import { usePolling } from "../composables/ui/usePolling"
import { useToast } from "../composables/useToast"
import { useTemplates } from "../composables/assistant/useTemplates"
import { useAssistant } from "../composables/assistant/useAssistant"
import { useRunningActivity } from "../composables/assistant/useRunningActivity"
import { useGlobalRunActivity } from "../composables/assistant/useGlobalRunActivity"
import { DispatcherService } from "../services/automation/dispatcherService"
import { isPathWithin } from "../utils/workspace/fileTree"
import {
  toWorkspace,
  toWorkspaceAssistant,
  toWorkspaceFile,
  toWorkspaces,
  toWorkspaceSection,
  workspaceLocation,
} from "../router/routes"
import type { AutomationRun } from "../types/dispatcher"
import type { LaneKind } from "../types/assistant"
import type { MemoryEntry } from "../types/memory"
import type { WorkspaceNavSection } from "../types/routes"

import WorkspaceList from "../components/AgentIde/workspace/WorkspaceList.vue"
import WorkspaceHeader from "../components/AgentIde/workspace/WorkspaceHeader.vue"
import WorkspaceFiles from "../components/AgentIde/workspace/WorkspaceFiles.vue"
import HistoricalRunDetails from "../components/AgentIde/automation/HistoricalRunDetails.vue"
import MemoryPanel from "../components/AgentIde/memory/MemoryPanel.vue"
import MemoryDetail from "../components/AgentIde/memory/MemoryDetail.vue"
import AssistantChat from "../components/AgentIde/assistant/AssistantChat.vue"
import WorkspaceSettings from "../components/AgentIde/workspace/WorkspaceSettings.vue"
import FileEditor from "../components/AgentIde/workspace/FileEditor.vue"
import TemplateLibrary from "../components/AgentIde/system/TemplateLibrary.vue"
import MonitorPanel from "../components/AgentIde/common/MonitorPanel.vue"
import ContextDrawer from "../components/layout/ContextDrawer.vue"
import PageHeader from "../components/common/layout/PageHeader.vue"
import Panel from "../components/common/layout/Panel.vue"
import BaseButton from "../components/common/buttons/BaseButton.vue"
import EmptyState from "../components/common/feedback/EmptyState.vue"
import ErrorState from "../components/common/feedback/ErrorState.vue"

// Workspaces (plan D9, D18, Phase 5): the list of workspaces, and one
// workspace's sections — Files (tree + editor), Assistant, Memory, Security,
// Playbooks — each a route. Kept alive across destinations (D19), so the
// editor buffer and chat survive a trip to another page.

const REFRESH_INTERVAL_MS = 10_000
// Chats queue behind automations, so an automation holder is the likely blocker.
const AUTOMATION_LANE_KIND: LaneKind = "automation"

const router = useRouter()
const current = useDestinationRoute("workspaces")
const location = computed(() => workspaceLocation(current.value))
const ws = computed(() => location.value.ws)

const {
  metrics,
  workspaces,
  workspaceTrees,
  loading,
  fetchMetrics,
  fetchWorkspaces,
  fetchWorkspaceTree,
  fetchWorkspaceState,
  fetchGlobalActivity,
  createWorkspace,
  deleteWorkspacePaths,
  deleteWorkspace,
  deleteRun,
  deleteAutomationRuns,
} = useDispatcher()
const { state: adminState } = useModels()
const toast = useToast()
const { runningSessions, reconcileRunning, reconcileRunningConversation } = useAssistant()
const { isMobile } = useResponsiveLayout()
const { workspaceHistory, refreshHistory } = useWorkspaceHistory()
const refreshWorkspaceHistory = () => refreshHistory(ws.value, fetchWorkspaceState, fetchGlobalActivity)

// Authoritative per-workspace "running" source. Drives the chat-menu glow and
// heals sticky local running flags when the backend reports nothing running.
const { assistantRunning, assistantConversationId, assistantQueued } = useRunningActivity(ws)
// Lane occupancy is global (one scheduler for every workspace), so it comes from
// the shared global source the header indicator also reads.
const { laneHolders } = useGlobalRunActivity()
watch(assistantRunning, (running) => reconcileRunning(running))
// When the backend reports which conversation is running, mark exactly that
// history row as running so the indicator survives a page refresh.
watch(assistantConversationId, (id) => reconcileRunningConversation(id))

// Label of the run occupying the lane while the assistant chat waits for a
// slot. The lane is global, so the blocker may live in another workspace: its
// label already carries the workspace, and naming it beats showing nothing.
const laneWaitingLabel = computed(() => {
  if (!assistantQueued.value) return ""
  const blocker = laneHolders.value.find((holder) => holder.kind === AUTOMATION_LANE_KIND) ?? laneHolders.value[0]
  return blocker?.label ?? ""
})

// Not addressable: a run opened from the history list, a memory entry.
const selectedRun = ref<AutomationRun | null>(null)
const selectedMemory = ref<MemoryEntry | null>(null)
const monitorOpen = ref(false)

const { activeMainView, explorerVisible, mainVisible } = useViewManager({
  location,
  selectedRun,
  selectedMemory,
  isMobile,
})

// ── Files ────────────────────────────────────────────────────────────────
// The buffer outlives section and destination changes; only opening another
// file replaces it, so only that (or closing it) asks about unsaved changes.
const {
  selectedFile,
  fileContent,
  loadingFile,
  savingFile,
  isDirty,
  handleOpenFile,
  handleSaveFile,
  handleCreateFile,
  closeFile,
} = useFileEditor(toast)

const isBufferOf = (workspace: string | null, path: string) =>
  selectedFile.value?.workspace === workspace && selectedFile.value?.filename === path

const { confirmLeave } = useUnsavedChangesGuard(isDirty, {
  discards: (to) => {
    const target = workspaceLocation(to)
    return !!target.filePath && !isBufferOf(target.ws, target.filePath)
  },
})

watch(
  () => [location.value.ws, location.value.filePath] as const,
  ([workspace, path]) => {
    if (workspace && path && !isBufferOf(workspace, path)) void handleOpenFile(workspace, path)
  },
  { immediate: true },
)

const openFile = (workspace: string, filename: string) => router.push(toWorkspaceFile(workspace, filename))

async function handleCloseEditor() {
  if (!(await confirmLeave())) return
  closeFile()
  await router.push(toWorkspaceFile(ws.value ?? ""))
}

// A new file appears in the tree and opens in the editor.
function handleCreateFileInTree(workspace: string, filename: string) {
  return handleCreateFile(workspace, filename, async (ws) => {
    await fetchWorkspaceTree(ws)
    await openFile(ws, filename)
  })
}

// Deleting a folder takes the open file along when it lies inside it.
async function handleDeletePaths(workspace: string, paths: string[]) {
  const deleted = await deleteWorkspacePaths(workspace, paths)
  const removed = (path: string) => deleted.some((gone) => isPathWithin(path, gone))
  const open = selectedFile.value?.workspace === workspace ? selectedFile.value.filename : null
  if (!open || !removed(open)) return
  closeFile()
  if (location.value.filePath && removed(location.value.filePath)) await router.push(toWorkspaceFile(workspace))
}

// ── Workspace selection ──────────────────────────────────────────────────
watch(
  ws,
  (workspace) => {
    selectedMemory.value = null
    if (workspace) void fetchWorkspaceTree(workspace)
    void refreshWorkspaceHistory()
  },
  { immediate: true },
)
// Any navigation closes a run opened from the history list.
watch(() => current.value.fullPath, () => {
  selectedRun.value = null
})

// The workspace list is loaded once per visit; an unknown :ws in the URL then
// says so instead of showing an empty workspace.
const workspacesLoaded = ref(false)
const unknownWorkspace = computed(() => !!ws.value && workspacesLoaded.value && !workspaces.value.some((w) => w.id === ws.value))

// The sidebar's assistant shortcut opens the workspace last viewed here; with a
// single workspace that one is remembered as soon as the list loads, and a
// remembered workspace that no longer exists is forgotten.
const { lastWorkspace, remember: rememberWorkspace, forget: forgetWorkspace } = useLastWorkspace()
watch([ws, workspacesLoaded], ([workspace, loaded]) => {
  if (!loaded) return
  if (workspace && !unknownWorkspace.value) rememberWorkspace(workspace)
  const ids = workspaces.value.map((w) => w.id)
  if (lastWorkspace.value && !ids.includes(lastWorkspace.value)) forgetWorkspace(lastWorkspace.value)
  if (!lastWorkspace.value && ids.length === 1) rememberWorkspace(ids[0]!)
}, { immediate: true })

const section = computed<WorkspaceNavSection>(() => (location.value.assistant ? "assistant" : location.value.section ?? "files"))

// Switching workspace keeps the section (a file path does not carry over).
function switchWorkspace(id: string) {
  const s = section.value
  if (s === "files") void router.push(toWorkspaceFile(id))
  else if (s === "assistant") void router.push(toWorkspaceAssistant(id))
  else void router.push(toWorkspaceSection(id, s))
}

async function handleCreateWorkspace(name: string) {
  try {
    await createWorkspace(name)
  } catch {
    return // the composable shows the banner
  }
  await router.push(toWorkspace(name))
}

async function handleDeleteWorkspace(id: string) {
  await deleteWorkspace(id)
  forgetWorkspace(id)
  if (id === ws.value) await router.push(toWorkspaces())
}

// Files and the editor share the page; on mobile one shows at a time.
const onFilesPage = computed(() => activeMainView.value === "overview" || activeMainView.value === "editor")
const bufferFile = computed(() => (selectedFile.value?.workspace === ws.value ? selectedFile.value?.filename ?? null : null))

function handleConversationChange(id: string | null) {
  if (ws.value) void router.replace(toWorkspaceAssistant(ws.value, id ?? undefined))
}

function openAssistantSession(sessionId: string) {
  if (!ws.value) return
  monitorOpen.value = false
  void router.push(toWorkspaceAssistant(ws.value, sessionId))
}

// ── Runs ─────────────────────────────────────────────────────────────────
function selectRun(run: AutomationRun) {
  monitorOpen.value = false
  selectedRun.value = run
}

// The delete composables already surface errors via a banner, so a thrown
// error means "did not happen": no refresh and no view change.
async function handleDeleteRun(run: AutomationRun, closeDetails = false) {
  try {
    await deleteRun(run)
  } catch {
    return
  }
  if (closeDetails) selectedRun.value = null
  void refreshWorkspaceHistory()
}

async function handleClearAutomationRuns(auto: { workspace: string; name: string }) {
  if (!auto.workspace || !auto.name) return
  try {
    await deleteAutomationRuns(auto.workspace, auto.name)
  } catch {
    return
  }
  selectedRun.value = null
  void refreshWorkspaceHistory()
}

// ── Playbooks ────────────────────────────────────────────────────────────
const { handleInjectTemplate } = useTemplates(ws, selectedFile, fileContent, fetchWorkspaceTree, openFile)

// ── External-access flags for the explorer ───────────────────────────────
const workspaceExternalAccess = ref<Record<string, boolean>>({})

async function refreshExternalAccess() {
  try {
    const configs = await DispatcherService.getAllWorkspaceConfigs()
    const access: Record<string, boolean> = {}
    for (const [wsId, cfg] of Object.entries(configs)) {
      const paths: unknown = cfg?.guardrails?.terminal?.allowed_external_paths
      if (Array.isArray(paths) && paths.length > 0) access[wsId] = true
    }
    workspaceExternalAccess.value = access
  } catch (err) {
    console.error("Failed to refresh external access flags", err)
  }
}

onMounted(async () => {
  void fetchMetrics()
  void refreshExternalAccess()
  await fetchWorkspaces()
  workspacesLoaded.value = true
})
usePolling(() => {
  void refreshWorkspaceHistory()
  void fetchMetrics()
}, REFRESH_INTERVAL_MS)
</script>

<template>
  <div class="flex flex-col gap-4">
    <!-- All workspaces -->
    <template v-if="!ws">
      <PageHeader eyebrow="Agents" title="Workspaces" subtitle="Each workspace holds task files, playbooks, memories and conversations for one agent.">
        <template #actions>
          <BaseButton variant="secondary" icon="nav-activity" :aria-expanded="monitorOpen" @click="monitorOpen = true">Monitor</BaseButton>
        </template>
      </PageHeader>
      <Panel title="All workspaces" flush>
        <WorkspaceList
          :workspaces="workspaces"
          :external-access="workspaceExternalAccess"
          :loading="!workspacesLoaded"
          @create="handleCreateWorkspace"
          @delete="handleDeleteWorkspace"
        />
      </Panel>
    </template>

    <!-- A workspace that does not exist -->
    <ErrorState
      v-else-if="unknownWorkspace"
      :title="`No workspace ${ws}`"
      cause="It may have been renamed or deleted."
      next="Pick another workspace from the list."
    >
      <template #action><RouterLink :to="toWorkspaces()" class="font-mono text-[length:var(--text-small)] text-accent-info-text hover:underline">All workspaces</RouterLink></template>
    </ErrorState>

    <!-- One workspace -->
    <template v-else>
      <WorkspaceHeader
        :ws="ws"
        :workspaces="workspaces"
        :section="section"
        :chat-running="assistantRunning"
        :external-access="!!workspaceExternalAccess[ws]"
        @switch="switchWorkspace"
      >
        <template #actions>
          <BaseButton variant="secondary" icon="nav-activity" :aria-expanded="monitorOpen" @click="monitorOpen = true">Monitor</BaseButton>
        </template>
      </WorkspaceHeader>

      <Panel v-if="activeMainView === 'history' && selectedRun" title="Run" flush>
        <HistoricalRunDetails
          :run="selectedRun"
          @close="selectedRun = null"
          @delete-run="(run: AutomationRun) => handleDeleteRun(run, true)"
          @delete-automation-runs="handleClearAutomationRuns"
        />
      </Panel>

      <!-- Files: tree + editor -->
      <div v-else-if="onFilesPage" class="grid gap-4 lg:grid-cols-[18rem_minmax(0,1fr)]">
        <Panel v-show="explorerVisible" title="Files">
          <WorkspaceFiles
            :workspace="ws"
            :tree="workspaceTrees[ws]"
            :selected-path="location.filePath || null"
            :loading="loading"
            class="max-h-[70vh]"
            @open-file="(path: string) => openFile(ws!, path)"
            @create-file="(path: string) => handleCreateFileInTree(ws!, path)"
            @delete-paths="(paths: string[]) => handleDeletePaths(ws!, paths)"
          />
        </Panel>

        <Panel v-show="mainVisible" :title="activeMainView === 'editor' && selectedFile ? selectedFile.filename : 'Editor'" :preserve-case="activeMainView === 'editor'" flush>
          <template v-if="activeMainView === 'editor' && selectedFile" #actions>
            <RouterLink :to="toWorkspaceFile(ws)" class="font-mono text-[length:var(--text-small)] text-muted hover:text-primary lg:hidden">← Files</RouterLink>
            <UnsavedTag v-if="isDirty" label="Unsaved" />
            <CopyButton :text="fileContent" title="Copy file contents" />
            <BaseButton size="sm" :loading="savingFile" :disabled="loadingFile || !isDirty" @click="handleSaveFile()">Save</BaseButton>
            <BaseButton variant="ghost" size="sm" icon="close" icon-only label="Close file" @click="handleCloseEditor" />
          </template>
          <FileEditor
            v-if="activeMainView === 'editor' && selectedFile"
            :filename="selectedFile.filename"
            :content="fileContent"
            :loading="loadingFile"
            @update:content="fileContent = $event"
            @save="handleSaveFile()"
          />
          <EmptyState
            v-else
            title="Open a file"
            body="Pick a file in the tree, or create one; nested paths such as notes/today.md are fine."
          />
        </Panel>
      </div>

      <div v-else-if="activeMainView === 'assistant'" class="h-[calc(100vh-10rem)] min-h-[480px] overflow-hidden rounded-[var(--radius-sm)] border border-hairline">
        <AssistantChat
          :workspaceId="ws"
          :conversation-id="location.conversationId"
          @update:conversation-id="handleConversationChange"
          @close="router.push(toWorkspaceFile(ws))"
        />
      </div>

      <Panel v-else-if="activeMainView === 'memory' || activeMainView === 'memory-detail'" title="Memory">
        <MemoryDetail
          v-if="activeMainView === 'memory-detail' && selectedMemory"
          :entry="selectedMemory"
          :workspace-id="ws"
          @close="selectedMemory = null"
          @updated="selectedMemory = null"
        />
        <MemoryPanel v-else :workspace-id="ws" @select-memory="(entry: MemoryEntry) => (selectedMemory = entry)" />
      </Panel>

      <WorkspaceSettings
        v-else-if="activeMainView === 'settings' && adminState"
        :workspace-id="ws"
        :global-guardrails="adminState.config.guardrails"
      />

      <Panel v-else-if="activeMainView === 'playbooks'" title="Playbooks">
        <TemplateLibrary :append-target="bufferFile" @inject="handleInjectTemplate" />
      </Panel>
    </template>

    <ContextDrawer v-model:open="monitorOpen" title="Monitor">
      <MonitorPanel
        :history="workspaceHistory"
        :loading="loading"
        :metrics="metrics"
        :assistant-sessions="runningSessions"
        :lane-waiting-label="laneWaitingLabel"
        @select-run="selectRun"
        @select-assistant-session="openAssistantSession"
        @delete-run="(run: AutomationRun) => handleDeleteRun(run)"
      />
    </ContextDrawer>
  </div>
</template>
