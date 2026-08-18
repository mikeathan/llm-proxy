<script setup lang="ts">
import { ref, watch, computed, nextTick, onMounted } from "vue";
import { useAssistant } from "../../../composables/assistant/useAssistant";
import { AssistantService } from "../../../services/assistant/assistantService";
import { groupTurns } from "../../../utils/message/turnGrouper";
import { useResponsiveLayout } from "../../../composables/ui/useResponsiveLayout";
import GuardrailBanner from "../../../components/common/chat/GuardrailBanner.vue";
import ChatSessionList from "./ChatSessionList.vue";
import ChatMessages from "./ChatMessages.vue";
import ChatInput from "./ChatInput.vue";
import Icon from "../../../components/icons/Icon.vue";
import { useTurnInset } from "../../../composables/ui/useTurnInset";

const props = defineProps<{
  workspaceId: string;
}>();

const emit = defineEmits<{
  (e: "close"): void;
}>();

const { isMobile } = useResponsiveLayout(640);

const {
  loading, messages, sessions, currentSessionId, pendingDecision,
  thinking, liveReasoning, paused, phase,
  fetchSessions, loadSession, newSession, sendMessage, deleteSession,
  deleteSessionsByIds, cancelSession, connectSSE, activeWorkspaceId, cancel,
  liveEvents,
} = useAssistant();

const inputMessage = ref("");
const sidebarOpen = ref(false);
const inboundCount = ref(0);
const chatMessagesRef = ref<InstanceType<typeof ChatMessages> | null>(null);

function forceScrollToBottom() {
  chatMessagesRef.value?.scrollToBottom("smooth");
}

// scrollToLatest waits for the DOM to reflect the latest state change, then
// scrolls to the bottom. Shared by the send path and session loading so the
// scroll-after-render dance lives in one place.
async function scrollToLatest() {
  await nextTick();
  forceScrollToBottom();
}

// launchRun is the single send path shared by the input box (handleSend) and
// the retry button (handleRetry): collapse every turn's work section (so a
// stale errored/cancelled bubble from a previous run cannot stay expanded and
// bleed the new run's live reasoning into it), send, then scroll to the new
// output after the DOM updates.
async function launchRun(text: string) {
  collapseAllInsets();
  await sendMessage(props.workspaceId, text);
  await scrollToLatest();
}

const currentSessionRunning = computed(() =>
  currentSessionId.value != null && sessions.value.some(s => s.id === currentSessionId.value && s.running)
)

watch(sidebarOpen, (open) => {
  if (open) inboundCount.value = 0
})

// Watches the liveEvents array length (O(1)) instead of deep-walking the whole
// unbounded history on every SSE append (O(n) per event, O(n²) over a long
// stream — the audit's flagged hot path). The callback only reads the last
// element, so the deep traversal was pure waste.
watch(() => liveEvents.value.length, () => {
  const last = liveEvents.value[liveEvents.value.length - 1]
  if (last && (last.payload as any)?.inbound) {
    inboundCount.value++
  }
})

function toggleSidebar() {
  sidebarOpen.value = !sidebarOpen.value;
  inboundCount.value = 0
}

const expandedSegments = ref<Record<string, boolean>>({});

const turns = computed(() => {
  return groupTurns(messages.value)
})
const { insetCollapsed, isInsetCollapsed, toggleInset, collapseAllInsets, resetInsets } = useTurnInset(phase, turns);
const lastMessageIsUser = computed(() => {
  if (messages.value.length === 0) return false;
  return messages.value[messages.value.length - 1]?.role === "user";
});

function isSegExpanded(turnIdx: number, segIdx: number): boolean {
  return !!expandedSegments.value[`${turnIdx}-${segIdx}`];
}

function toggleSegment(turnIdx: number, segIdx: number) {
  const key = `${turnIdx}-${segIdx}`;
  expandedSegments.value = { ...expandedSegments.value, [key]: !expandedSegments.value[key] };
}

onMounted(() => { if (props.workspaceId) initWorkspace(); });
watch(() => props.workspaceId, () => initWorkspace());

const initWorkspace = async () => {
  activeWorkspaceId.value = props.workspaceId;
  newSession();
  await fetchSessions(props.workspaceId);
  connectSSE();
};

  const handleNewChat = async () => {
    newSession();
    resetInsets();
    await fetchSessions(props.workspaceId);
  };

  const handleSend = async () => {
    const text = inputMessage.value.trim();
    if (!text || loading.value) return;
    inputMessage.value = "";
    await launchRun(text);
  };

const handleRetry = (text: string) => {
  void launchRun(text);
};

  const handleLoadSession = async (sessionId: string) => {
    resetInsets();
    await loadSession(props.workspaceId, sessionId);
    collapseAllInsets();
    await scrollToLatest();
    if (isMobile.value) {
      sidebarOpen.value = false;
    }
  };

const handleDeleteSession = async (sessionId: string) => {
  if (confirm("Are you sure you want to delete this conversation?")) {
    await deleteSession(props.workspaceId, sessionId);
  }
};

const handleCancelSession = async (sessionId: string) => {
  await cancelSession(props.workspaceId, sessionId)
}

const handleRenameSession = async (sessionId: string, title: string) => {
  try {
    await AssistantService.renameSession(props.workspaceId, sessionId, title);
    await fetchSessions(props.workspaceId);
  } catch (err) {
    console.error("Failed to rename session", err);
  }
};

const handleClearAll = async () => {
  if (!confirm("Delete all conversations in this workspace? This cannot be undone.")) return;
  try {
    await AssistantService.deleteAllSessions(props.workspaceId);
    await fetchSessions(props.workspaceId);
  } catch (err) {
    console.error("Failed to clear all sessions", err);
  }
};

const handleDeleteGroup = async (ids: string[]) => {
  if (!confirm("Delete all conversations in this group? This cannot be undone.")) return;
  await deleteSessionsByIds(props.workspaceId, ids);
};
</script>

<template>
  <div class="assistant-shell">
    <!-- Desktop sidebar -->
    <aside
      v-if="!isMobile"
      class="chat-sidebar"
      :class="{ 'chat-sidebar--open': sidebarOpen }"
    >
      <ChatSessionList
        v-if="sidebarOpen"
        :sessions="sessions"
        :current-session-id="currentSessionId"
        :is-mobile="false"
        @load="handleLoadSession"
        @delete="handleDeleteSession"
        @rename="handleRenameSession"
        @cancel="handleCancelSession"
        @new-chat="handleNewChat"
        @clear-all="handleClearAll"
        @delete-group="handleDeleteGroup"
        @close="toggleSidebar"
      />
    </aside>

    <!-- Mobile drawer -->
    <Transition name="drawer">
      <div v-if="isMobile && sidebarOpen" class="mobile-drawer" @click.stop>
        <ChatSessionList
          :sessions="sessions"
          :current-session-id="currentSessionId"
          :is-mobile="true"
          @load="handleLoadSession"
          @delete="handleDeleteSession"
          @rename="handleRenameSession"
          @cancel="handleCancelSession"
          @new-chat="handleNewChat"
          @clear-all="handleClearAll"
          @delete-group="handleDeleteGroup"
          @close="toggleSidebar"
        />
      </div>
    </Transition>

    <!-- Mobile backdrop -->
    <Transition name="fade">
      <div
        v-if="isMobile && sidebarOpen"
        class="mobile-backdrop"
        @click="toggleSidebar"
      />
    </Transition>

    <div class="chat-area">
      <header class="chat-header">
        <div class="flex items-center gap-2">
          <button @click="toggleSidebar" class="btn-header-action relative" :title="sidebarOpen ? 'Hide conversations' : 'Show conversations'">
            <Icon :name="sidebarOpen ? 'chevron-left' : 'chevron-right'" size="sm" />
            <span v-if="inboundCount > 0 && !sidebarOpen" class="badge-dot" />
          </button>
          <button @click="handleNewChat" class="btn-header-action" title="New Chat">
            <Icon name="plus" size="sm" />
          </button>
          <div class="chat-info">
            <span class="chat-status">Agent Online</span>
            <h2 class="chat-title">{{ workspaceId }}</h2>
          </div>
        </div>
        <button @click="emit('close')" class="btn-chat-close" title="Exit Chat">
          <Icon name="close" size="sm" />
        </button>
      </header>

      <div v-if="pendingDecision" class="guardrail-banner-wrapper">
        <GuardrailBanner :decision="pendingDecision" @allow="(..._args: any[]) => {}" @deny="() => {}" />
      </div>

      <div v-if="messages.length === 0 && currentSessionRunning && !loading" class="chat-processing-banner">
        Agent is processing…
      </div>

      <ChatMessages
        ref="chatMessagesRef"
        :messages="messages"
        :turns="turns"
        :loading="loading"
        :thinking="thinking"
        :live-reasoning="liveReasoning"
        :paused="paused"
        :last-message-is-user="lastMessageIsUser"
        :workspace-id="workspaceId"
        :turns-collapsed="insetCollapsed"
        :expanded-segments="expandedSegments"
        :is-inset-collapsed="isInsetCollapsed"
        :is-seg-expanded="isSegExpanded"
        :phase="phase"
        @retry="handleRetry"
        @toggle-inset="toggleInset"
        @toggle-segment="toggleSegment"
      />

      <ChatInput
        :loading="loading"
        :paused="paused"
        :input-message="inputMessage"
        @send="handleSend"
        @cancel="cancel"
        @update:input-message="inputMessage = $event"
      />
    </div>
  </div>
</template>

<style scoped lang="postcss">
@import url('../../../styles/theme.css');

.assistant-shell {
  @apply h-full flex overflow-hidden relative;
}

/* ── Desktop sidebar ── */
.chat-sidebar {
  @apply shrink-0 transition-[width] duration-200 ease-out overflow-hidden;
  width: 0;
}

.chat-sidebar--open {
  width: 260px;
}

/* ── Mobile drawer + backdrop ── */
.mobile-backdrop {
  @apply fixed inset-0 bg-black/50 z-30 backdrop-blur-sm;
}

.mobile-drawer {
  @apply fixed left-0 top-0 bottom-0 w-[85vw] max-w-[320px] z-40;
}

/* ── Drawer transitions ── */
.drawer-enter-active {
  transition: transform 250ms cubic-bezier(0.4, 0, 0.2, 1);
}
.drawer-leave-active {
  transition: transform 200ms cubic-bezier(0.4, 0, 0.2, 1);
}
.drawer-enter-from,
.drawer-leave-to {
  transform: translateX(-100%);
}
.drawer-enter-to,
.drawer-leave-from {
  transform: translateX(0);
}

/* ── Backdrop fade ── */
.fade-enter-active,
.fade-leave-active {
  transition: opacity 200ms ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

/* ── Chat area ── */
.chat-area {
  @apply flex-1 flex flex-col bg-gray-900 relative min-w-0;
}

.chat-header {
  @apply px-4 py-3 bg-gray-800/40 border-b border-white/5 flex items-center justify-between z-10;
}

.chat-info {
  @apply flex flex-col;
}

.chat-status {
  @apply text-[9px] font-bold text-green-500 uppercase tracking-widest leading-none mb-1;
}

.chat-title {
  @apply text-xs font-bold text-gray-200 leading-none;
}

.btn-header-action {
  @apply p-1.5 rounded-md hover:bg-gray-700 border border-gray-700 shadow-sm text-gray-400 hover:text-gray-200 transition-all duration-150 flex items-center justify-center focus:outline-none;
}
.btn-header-action:active {
  @apply scale-95;
}

.badge-dot {
  @apply absolute -top-0.5 -right-0.5 w-2 h-2 bg-red-500 rounded-full;
}

.btn-chat-close {
  @apply p-1.5 rounded-md hover:bg-red-600/30 text-gray-500 hover:text-red-400 transition-all duration-150;
}
.btn-chat-close:active {
  @apply scale-95;
}

.guardrail-banner-wrapper {
  @apply px-4 py-2;
}

.chat-processing-banner {
  @apply px-6 py-3 text-xs text-gray-500 italic;
}
</style>
