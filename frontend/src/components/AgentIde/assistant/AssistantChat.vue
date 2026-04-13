<script setup lang="ts">
import { ref, onMounted, watch, computed } from "vue";
import { useAssistant } from "../../../composables/useAssistant";
import { marked } from "marked";

const props = defineProps<{
  workspaceId: string;
}>();

const {
  loading,
  error,
  messages,
  sessions,
  currentSessionId,
  fetchSessions,
  loadSession,
  newSession,
  sendMessage,
  deleteSession,
  activeWorkspaceId,
} = useAssistant();

const inputMessage = ref("");
const messageContainer = ref<HTMLElement | null>(null);

onMounted(() => {
  if (props.workspaceId) {
    initWorkspace();
  }
});

watch(
  () => props.workspaceId,
  () => {
    initWorkspace();
  },
);

const initWorkspace = async () => {
  if (
    activeWorkspaceId.value === props.workspaceId &&
    messages.value.length > 0
  ) {
    // Already in this workspace and have a conversation active, just refresh sessions list
    await fetchSessions(props.workspaceId);
    scrollToBottom();
    return;
  }

  activeWorkspaceId.value = props.workspaceId;
  newSession();
  await fetchSessions(props.workspaceId);
};

const handleSend = async () => {
  const text = inputMessage.value.trim();
  if (!text || loading.value) return;

  inputMessage.value = "";
  await sendMessage(props.workspaceId, text);
  scrollToBottom();
};

const handleLoadSession = async (sessionId: string) => {
  await loadSession(props.workspaceId, sessionId);
  scrollToBottom();
};

const handleDeleteSession = async (sessionId: string) => {
  if (confirm("Are you sure you want to delete this conversation?")) {
    await deleteSession(props.workspaceId, sessionId);
  }
};

const scrollToBottom = () => {
  setTimeout(() => {
    if (messageContainer.value) {
      messageContainer.value.scrollTop = messageContainer.value.scrollHeight;
    }
  }, 100);
};

const renderMarkdown = (content: string) => {
  return marked.parse(content) as string;
};

// Formatter for timestamps
const formatTime = (isoString?: string) => {
  if (!isoString) return "";
  const d = new Date(isoString);
  return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
};

const isLastMessageUser = computed(() => {
  if (messages.value.length === 0) return false;
  const lastMessage = messages.value[messages.value.length - 1];
  return lastMessage?.role === "user";
});
</script>

<template>
  <div class="assistant-shell">
    <!-- Left Sidebar: Session List -->
    <div class="session-sidebar">
      <div class="sidebar-header">
        <h3 class="sidebar-title">Conversations</h3>
        <button @click="initWorkspace" class="btn-new" title="New Chat">
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
              d="M12 4v16m8-8H4"
            />
          </svg>
        </button>
      </div>

      <div class="session-list">
        <div v-if="sessions.length === 0" class="empty-sessions">
          No history in this workspace.
        </div>
        <div
          v-for="session in sessions"
          :key="session.id"
          class="session-row group"
        >
          <button
            @click="handleLoadSession(session.id)"
            class="session-item"
            :class="{ 'session-item--active': currentSessionId === session.id }"
          >
            <div class="session-snippet">
              {{ session.snippet || "Empty conversation" }}
            </div>
            <div class="session-time">{{ formatTime(session.updated_at) }}</div>
          </button>

          <button
            @click.stop="handleDeleteSession(session.id)"
            class="btn-delete"
            title="Delete conversation"
          >
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
                d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
              />
            </svg>
          </button>
        </div>
      </div>
    </div>

    <!-- Main Chat Area -->
    <div class="chat-area">
      <!-- Error Banner -->
      <div v-if="error" class="chat-error">
        {{ error }}
      </div>

      <!-- Messages -->
      <div class="message-container" ref="messageContainer">
        <div v-if="messages.length === 0" class="chat-empty">
          <div class="welcome-icon">💬</div>
          <h3>Workspace Assistant</h3>
          <p>
            You are talking to the agent bounded to
            <strong>{{ workspaceId }}</strong
            >.
          </p>
          <p>Ask it to scan files, check metrics, or help you debug.</p>
        </div>

        <div
          v-for="(msg, idx) in messages"
          :key="idx"
          class="message-wrapper"
          :class="`message-wrapper--${msg.role}`"
        >
          <div class="message-bubble" :class="`message-bubble--${msg.role}`">
            <div class="message-role">
              {{ msg.role === "user" ? "You" : "Assistant" }}
            </div>
            <div
              class="message-content markdown-body"
              v-html="renderMarkdown(msg.content)"
            ></div>
          </div>
        </div>

        <div
          v-if="loading && isLastMessageUser"
          class="message-wrapper message-wrapper--assistant"
        >
          <div
            class="message-bubble message-bubble--assistant typing-indicator"
          >
            <span></span><span></span><span></span>
          </div>
        </div>
      </div>

      <!-- Input Area -->
      <div class="input-area">
        <textarea
          v-model="inputMessage"
          @keydown.enter.exact.prevent="handleSend"
          placeholder="Ask the workspace agent..."
          class="chat-input"
          rows="1"
          :disabled="loading"
        ></textarea>
        <button
          @click="handleSend"
          :disabled="!inputMessage.trim() || loading"
          class="btn-send"
        >
          <svg
            xmlns="http://www.w3.org/2000/svg"
            class="h-5 w-5"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M12 19l9 2-9-18-9 18 9-2zm0 0v-8"
            />
          </svg>
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.assistant-shell {
  @apply h-full flex bg-gray-800 rounded-lg overflow-hidden border border-white/5;
}

/* Sidebar */
.session-sidebar {
  @apply w-64 border-r border-gray-700 flex flex-col bg-gray-800/50;
}

.sidebar-header {
  @apply p-4 border-b border-gray-700 flex items-center justify-between shrink-0;
}

.sidebar-title {
  @apply text-xs font-bold text-gray-400 uppercase tracking-widest;
}

.btn-new {
  @apply p-1.5 rounded-md hover:bg-gray-700 text-gray-400 hover:text-white transition-colors;
}

.session-list {
  @apply flex-1 overflow-y-auto p-2 flex flex-col gap-1;
}

.empty-sessions {
  @apply p-4 text-xs text-center text-gray-500 italic;
}

.session-row {
  @apply relative flex items-center w-full px-2 overflow-hidden;
}

.session-item {
  @apply flex-1 text-left p-3 pr-12 rounded-md transition-all flex flex-col gap-1 border border-transparent min-w-0;
  @apply hover:bg-white/5;
}

.session-item--active {
  @apply bg-blue-600/10 border-blue-500/30;
}

.btn-delete {
  @apply absolute right-4 top-1/2 -translate-y-1/2 p-2 rounded-lg text-gray-500 opacity-0 transition-all scale-95;
  @apply hover:bg-red-500/15 hover:text-red-400;
  @apply flex items-center justify-center;
}

.session-row:hover .btn-delete {
  @apply opacity-100 scale-100;
}

.session-snippet {
  @apply text-sm text-gray-200 truncate font-medium block w-full;
}

.session-time {
  @apply text-[10px] text-gray-500 font-mono;
}

/* Chat Area */
.chat-area {
  @apply flex-1 flex flex-col bg-gray-900 relative;
}

.chat-error {
  @apply absolute top-0 left-0 right-0 z-10 bg-red-900/90 text-red-200 p-2 text-xs text-center font-medium;
}

.message-container {
  @apply flex-1 overflow-y-auto p-4 md:p-6 flex flex-col gap-5;
}

.chat-empty {
  @apply m-auto flex flex-col items-center justify-center text-center text-gray-500 gap-3 max-w-sm px-6;
}

.welcome-icon {
  @apply text-4xl mb-2 opacity-50;
}

.message-wrapper {
  @apply flex w-full;
}

.message-wrapper--user {
  @apply justify-end;
}

.message-wrapper--assistant {
  @apply justify-start;
}

.message-bubble {
  @apply max-w-[95%] sm:max-w-[85%] rounded-2xl p-4 flex flex-col gap-1 shadow-sm;
}

.message-bubble--user {
  @apply bg-blue-600 text-white rounded-tr-sm;
}

.message-bubble--assistant {
  @apply bg-gray-800 text-gray-200 border border-gray-700 rounded-tl-sm;
}

.message-role {
  @apply text-[10px] font-bold uppercase tracking-wider opacity-60;
}

.message-content {
  @apply text-sm leading-relaxed;
}

/* Base markdown styles to ensure tables/code blocks fit */
:deep(.markdown-body p) {
  @apply mb-2 last:mb-0;
}

:deep(.markdown-body pre) {
  @apply bg-gray-900 p-3 rounded mt-2 mb-2 overflow-x-auto text-xs;
}

:deep(.markdown-body code) {
  @apply bg-gray-900 px-1 py-0.5 rounded text-xs text-blue-300;
}

:deep(.markdown-body a) {
  @apply text-blue-400 hover:underline;
}

:deep(.markdown-body ul) {
  @apply list-disc list-inside mt-1 mb-2;
}

:deep(.markdown-body ol) {
  @apply list-decimal list-inside mt-1 mb-2;
}

/* Input Area */
.input-area {
  @apply p-4 border-t border-gray-700 bg-gray-800 flex gap-2 shrink-0;
}

.chat-input {
  @apply flex-1 bg-gray-900 border border-gray-700 rounded-xl px-4 py-3 text-sm text-gray-200 
         focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500 resize-none
         placeholder-gray-600 transition-all;
}

.chat-input:disabled {
  @apply opacity-50 cursor-not-allowed;
}

.btn-send {
  @apply bg-blue-600 hover:bg-blue-500 text-white rounded-xl px-4 flex items-center justify-center
         transition-colors disabled:opacity-50 disabled:cursor-not-allowed shadow-md w-14 shrink-0;
}

/* Typing Indicator */
.typing-indicator {
  @apply flex flex-row gap-1 items-center px-5 py-4;
}

.typing-indicator span {
  @apply w-1.5 h-1.5 bg-gray-500 rounded-full animate-pulse;
}

.typing-indicator span:nth-child(2) {
  animation-delay: 0.2s;
}

.typing-indicator span:nth-child(3) {
  animation-delay: 0.4s;
}
</style>
