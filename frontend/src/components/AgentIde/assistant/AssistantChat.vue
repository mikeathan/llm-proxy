<script setup lang="ts">
import { ref, watch, computed, nextTick, onMounted } from "vue";
import { useAssistant } from "../../../composables/assistant/useAssistant";
import { AssistantService } from "../../../services/assistantService";
import { groupTurns } from "../../../utils/message/turnGrouper";
import { useResponsiveLayout } from "../../../composables/ui/useResponsiveLayout";
import GuardrailBanner from "../../../components/common/chat/GuardrailBanner.vue";
import ChatSessionList from "./ChatSessionList.vue";
import ChatMessages from "./ChatMessages.vue";
import ChatInput from "./ChatInput.vue";
import Icon from "../../../components/icons/Icon.vue";

const props = defineProps<{
  workspaceId: string;
}>();

const emit = defineEmits<{
  (e: "close"): void;
}>();

const { isMobile } = useResponsiveLayout(640);

const {
  loading, error, messages, sessions, currentSessionId, pendingDecision,
  thinking, liveReasoning, paused,
  fetchSessions, loadSession, newSession, sendMessage, deleteSession,
  activeWorkspaceId, cancel,
} = useAssistant();

const dismissError = () => { error.value = null }

const inputMessage = ref("");
const sidebarOpen = ref(false);

function toggleSidebar() {
  sidebarOpen.value = !sidebarOpen.value;
}

const workCollapsed = ref<Record<number, boolean>>({});
const expandedSegments = ref<Record<string, boolean>>({});

const turns = computed(() => groupTurns(messages.value));
const lastMessageIsUser = computed(() => {
  if (messages.value.length === 0) return false;
  return messages.value[messages.value.length - 1]?.role === "user";
});

function isWorkCollapsed(turnIdx: number): boolean {
  return !!workCollapsed.value[turnIdx];
}

function isSegExpanded(turnIdx: number, segIdx: number): boolean {
  return !!expandedSegments.value[`${turnIdx}-${segIdx}`];
}

function toggleWork(turnIdx: number) {
  const current = !!workCollapsed.value[turnIdx];
  workCollapsed.value = { ...workCollapsed.value, [turnIdx]: !current };
}

function toggleSegment(turnIdx: number, segIdx: number) {
  const key = `${turnIdx}-${segIdx}`;
  expandedSegments.value = { ...expandedSegments.value, [key]: !expandedSegments.value[key] };
}

function collapseAllWork() {
  const collapsed: Record<number, boolean> = {};
  turns.value.forEach((_, idx) => { collapsed[idx] = true; });
  workCollapsed.value = collapsed;
}

onMounted(() => { if (props.workspaceId) initWorkspace(); });
watch(() => props.workspaceId, () => initWorkspace());

const initWorkspace = async () => {
  activeWorkspaceId.value = props.workspaceId;
  newSession();
  await fetchSessions(props.workspaceId);
};

const handleNewChat = async () => {
  newSession();
  workCollapsed.value = {};
  await fetchSessions(props.workspaceId);
};

const handleSend = async () => {
  const text = inputMessage.value.trim();
  if (!text || loading.value) return;
  inputMessage.value = "";
  collapseAllWork();
  await sendMessage(props.workspaceId, text);
  await nextTick();
  scrollToBottom();
};

const handleRetry = (text: string) => {
  sendMessage(props.workspaceId, text);
  scrollToBottom();
};

const handleLoadSession = async (sessionId: string) => {
  workCollapsed.value = {};
  await loadSession(props.workspaceId, sessionId);
  await nextTick();
  collapseAllWork();
  scrollToBottom();
  if (isMobile.value) {
    sidebarOpen.value = false;
  }
};

const handleDeleteSession = async (sessionId: string) => {
  if (confirm("Are you sure you want to delete this conversation?")) {
    await deleteSession(props.workspaceId, sessionId);
  }
};

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

function scrollToBottom() {
  const el = document.querySelector(".message-container");
  if (el && el.scrollHeight - el.scrollTop - el.clientHeight > 80) return;
  if (el) el.scrollTop = el.scrollHeight;
}

watch(loading, async (newVal, oldVal) => {
  if (!oldVal && newVal) {
    await nextTick();
    const lastIdx = turns.value.length - 1;
    if (lastIdx >= 0) {
      workCollapsed.value = { ...workCollapsed.value, [lastIdx]: false };
    }
  }
  if (oldVal && !newVal) {
    await nextTick();
    collapseAllWork();
  }
});
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
        @new-chat="handleNewChat"
        @clear-all="handleClearAll"
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
          @new-chat="handleNewChat"
          @clear-all="handleClearAll"
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
          <button @click="toggleSidebar" class="btn-header-action" :title="sidebarOpen ? 'Hide conversations' : 'Show conversations'">
            <Icon :name="sidebarOpen ? 'chevron-left' : 'chevron-right'" size="sm" />
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

      <ChatMessages
        :messages="messages"
        :turns="turns"
        :loading="loading"
        :thinking="thinking"
        :live-reasoning="liveReasoning"
        :paused="paused"
        :last-message-is-user="lastMessageIsUser"
        :workspace-id="workspaceId"
        :turns-collapsed="workCollapsed"
        :expanded-segments="expandedSegments"
        :is-work-collapsed="isWorkCollapsed"
        :is-seg-expanded="isSegExpanded"
        :error="error"
        @retry="handleRetry"
        @toggle-work="toggleWork"
        @toggle-segment="toggleSegment"
        @dismiss-error="dismissError"
      />

      <ChatInput
        :loading="loading"
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
  @apply shrink-0 transition-all duration-200 ease-out overflow-hidden;
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

.btn-chat-close {
  @apply p-1.5 rounded-md hover:bg-red-600/30 text-gray-500 hover:text-red-400 transition-all duration-150;
}
.btn-chat-close:active {
  @apply scale-95;
}

.guardrail-banner-wrapper {
  @apply px-4 py-2;
}
</style>
