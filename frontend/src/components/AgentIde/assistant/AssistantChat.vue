<script setup lang="ts">
import { ref, watch, computed, nextTick, onMounted, onUnmounted } from "vue";
import { useAssistant } from "../../../composables/useAssistant";
import { AssistantService } from "../../../services/assistantService";
import { groupTurns } from "../../../utils/turnGrouper";
import GuardrailBanner from "../../../components/common/GuardrailBanner.vue";
import ChatSessionList from "./ChatSessionList.vue";
import ChatMessages from "./ChatMessages.vue";
import ChatInput from "./ChatInput.vue";
import Icon from "../../../components/icons/Icon.vue";

const props = defineProps<{
  workspaceId: string;
  overlay?: boolean;
}>();

const emit = defineEmits<{
  (e: "close"): void;
}>();

const {
  loading, error, messages, sessions, currentSessionId, pendingDecision,
  thinking, liveReasoning, paused,
  fetchSessions, loadSession, newSession, sendMessage, deleteSession,
  activeWorkspaceId, cancel,
} = useAssistant();

const dismissError = () => { error.value = null }

const inputMessage = ref("");
const sidebarCollapsed = ref(true);
const isMobile = ref(window.innerWidth < 640);
function onResize() { isMobile.value = window.innerWidth < 640; }
if (typeof window !== 'undefined') {
  window.addEventListener('resize', onResize);
  onUnmounted(() => window.removeEventListener('resize', onResize));
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
    <ChatSessionList
      v-if="!sidebarCollapsed && !isMobile"
      :sessions="sessions"
      :current-session-id="currentSessionId"
      @load="handleLoadSession"
      @delete="handleDeleteSession"
      @rename="handleRenameSession"
      @new-chat="handleNewChat"
      @clear-all="handleClearAll"
    />

    <div class="chat-area">
      <header class="chat-header">
        <div class="flex items-center gap-3">
          <button @click="handleNewChat" class="btn-header-action" title="New Chat">
            <Icon name="plus" size="sm" />
          </button>
          <button @click="sidebarCollapsed = !sidebarCollapsed" class="btn-header-action" :title="sidebarCollapsed ? 'Show Conversations' : 'Hide Conversations'">
            <Icon name="chevron-double-right" size="sm" />
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

<style scoped>
@import url('../../../styles/theme.css');

.assistant-shell { @apply h-full flex bg-gray-800 rounded-lg overflow-hidden border border-white/5; }
.chat-area { @apply flex-1 flex flex-col bg-gray-900 relative; }
.chat-header { @apply px-4 py-3 border-b border-gray-700 bg-gray-800/80 flex items-center justify-between backdrop-blur-sm z-10; }
.chat-info { @apply flex flex-col; }
.chat-status { @apply text-[9px] font-bold text-green-500 uppercase tracking-widest leading-none mb-1; }
.chat-title { @apply text-xs font-bold text-gray-200 leading-none; }
.btn-header-action { @apply p-1.5 rounded-md hover:bg-gray-700 border border-gray-700 shadow-sm text-gray-400 hover:text-gray-200 transition-colors flex items-center justify-center focus:outline-none; }
.btn-chat-close { @apply p-1.5 rounded-md hover:bg-red-600/30 text-gray-500 hover:text-red-400 transition-all; }
.guardrail-banner-wrapper { @apply px-4 py-2; }
</style>
