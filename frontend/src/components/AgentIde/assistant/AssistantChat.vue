<script setup lang="ts">
import { ref, onMounted, watch, computed } from "vue";
import { useAssistant } from "../../../composables/useAssistant";
import { marked } from "marked";
import { formatTime } from "../../../utils/time";
import { getRoleLabel } from "../../../domain/assistant";
import { getToolCallPayload, getToolResPayload } from "../../../utils/dispatcher";
import ToolCallBlock from "../../../components/common/ToolCallBlock.vue";
import ToolResultBlock from "../../../components/common/ToolResultBlock.vue";
import GuardrailBanner from "../../../components/common/GuardrailBanner.vue";
import LifecycleMessage from "../../../components/common/LifecycleMessage.vue";
import CollapsiblePanel from "../../../components/common/CollapsiblePanel.vue";
import Icon from "../../../components/icons/Icon.vue";

const props = defineProps<{
  workspaceId: string;
}>();

const emit = defineEmits<{
  (e: "close"): void;
}>();

const {
  loading,
  error,
  messages,
  sessions,
  currentSessionId,
  streamingContent,
  liveEvents,
  pendingDecision,
  fetchSessions,
  loadSession,
  newSession,
  sendMessage,
  deleteSession,
  activeWorkspaceId,
} = useAssistant();

const inputMessage = ref("");
const messageContainer = ref<HTMLElement | null>(null);
const sidebarCollapsed = ref(true);

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

const isLastMessageUser = computed(() => {
  if (messages.value.length === 0) return false;
  const lastMessage = messages.value[messages.value.length - 1];
  return lastMessage?.role === "user";
});

const hasStreamingContent = computed(() => streamingContent.value.length > 0);

const toolCallEvents = computed(() => liveEvents.value.filter(ev => ev.type === 'tool_call'))
const toolResultEvents = computed(() => liveEvents.value.filter(ev => ev.type === 'tool_result'))
const lifecycleEvents = computed(() => liveEvents.value.filter(ev => ev.type === 'lifecycle'))

const toolResultContent = (tr: any): string => {
  if (!tr) return ""
  if (typeof tr.result === "string") return tr.result
  if (typeof tr.result === "object" && tr.result !== null) {
    if (typeof tr.result.content === "string") return tr.result.content
    return JSON.stringify(tr.result, null, 2)
  }
  return ""
}

const hasRawData = (tr: any): boolean => {
  if (!tr) return false
  if (typeof tr.result === "string") return false
  if (typeof tr.result === "object" && tr.result !== null && typeof tr.result.content === "string") return true
  return typeof tr.result === "object" && tr.result !== null
}

const toolResultRaw = (tr: any): string => {
  if (!tr) return ""
  return JSON.stringify(tr.result, null, 2)
}
</script>

<template>
  <div class="assistant-shell">
    <!-- Left Sidebar: Session List (collapsible) -->
    <CollapsiblePanel
      :collapsed="sidebarCollapsed"
      title="Conversations"
      position="left"
      @toggle="sidebarCollapsed = !sidebarCollapsed"
    >
      <template #header-actions>
        <button @click="initWorkspace" class="btn-new" title="New Chat">
          <Icon name="plus" size="sm" />
        </button>
      </template>

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
            <div class="session-time">{{ formatTime(session.updated_at || '') }}</div>
          </button>

          <button
            @click.stop="handleDeleteSession(session.id)"
            class="btn-delete"
            title="Delete conversation"
          >
            <Icon name="trash" size="xs" />
          </button>
        </div>
      </div>
    </CollapsiblePanel>

    <!-- Main Chat Area -->
    <div class="chat-area">
      <header class="chat-header">
        <div class="chat-info">
          <span class="chat-status">Agent Online</span>
          <h2 class="chat-title">{{ workspaceId }}</h2>
        </div>
        <button @click="emit('close')" class="btn-chat-close" title="Exit Chat">
          <Icon name="close" size="sm" />
        </button>
      </header>

      <!-- Error Banner -->
      <div v-if="error" class="chat-error">
        {{ error }}
      </div>

      <!-- Guardrail Banner -->
      <div v-if="pendingDecision" class="guardrail-banner-wrapper">
        <GuardrailBanner :decision="pendingDecision" @allow="(..._args: any[]) => {}" @deny="() => {}" />
      </div>

      <!-- Messages -->
      <div class="message-container" ref="messageContainer">
        <div v-if="messages.length === 0 && !loading && !hasStreamingContent" class="chat-empty">
          <div class="welcome-icon">💬</div>
          <h3>Workspace Assistant</h3>
          <p>
            You are talking to the agent bounded to
            <strong>{{ workspaceId }}</strong
            >.
          </p>
          <p>Ask it to scan files, check metrics, or help you debug.</p>
        </div>

        <!-- Existing messages -->
        <div
          v-for="(msg, idx) in messages"
          :key="'msg-' + idx"
          class="message-wrapper"
          :class="`message-wrapper--${msg.role}`"
        >
          <!-- Tool messages: render as structured result blocks -->
          <div v-if="msg.role === 'tool'" class="message-bubble message-bubble--tool">
            <div class="message-role">Tool Result</div>
            <div class="message-content markdown-body" v-html="renderMarkdown(toolResultContent(msg.toolResult))"></div>
            <details class="tool-result-details" v-if="hasRawData(msg.toolResult)">
              <summary class="tool-result-summary">
                <span class="tool-result-hint">(view raw)</span>
              </summary>
              <pre class="tool-result-body">{{ toolResultRaw(msg.toolResult) }}</pre>
            </details>
          </div>

          <!-- Assistant messages with tool calls: render inline -->
          <div v-else-if="msg.role === 'assistant' && msg.tool_calls && msg.tool_calls.length > 0" class="message-bubble message-bubble--assistant">
            <div class="message-role">Assistant</div>
            <div v-if="msg.content" class="message-content markdown-body" v-html="renderMarkdown(msg.content)"></div>
            <div v-for="(tc, tci) in msg.tool_calls" :key="'tc-' + tci" class="tool-call-inline">
              <ToolCallBlock :name="tc.function.name" :args="tc.function.arguments" />
            </div>
          </div>

          <!-- Normal assistant/user messages -->
          <div v-else class="message-bubble" :class="`message-bubble--${msg.role}`">
            <div class="message-role">
              {{ getRoleLabel(msg.role).replace(':', '') }}
            </div>
            <div
              v-if="msg.content"
              class="message-content markdown-body"
              v-html="renderMarkdown(msg.content)"
            ></div>
          </div>
        </div>

        <!-- Live events from SSE (tool calls, results, lifecycle) -->
        <!-- Each event type rendered independently — no v-else-if, so unknown types render nothing -->
        <ToolCallBlock
          v-for="(ev, idx) in toolCallEvents"
          :key="'tc-' + idx"
          :name="getToolCallPayload(ev).function.name"
          :args="getToolCallPayload(ev).function.arguments"
        />
        <ToolResultBlock
          v-for="(ev, idx) in toolResultEvents"
          :key="'tr-' + idx"
          :name="getToolResPayload(ev).name"
          :result="getToolResPayload(ev).result"
          :error="getToolResPayload(ev).error"
        />
        <LifecycleMessage
          v-for="(ev, idx) in lifecycleEvents"
          :key="'lc-' + idx"
          :phase="(ev.payload as any).phase"
          :payload="(ev.payload as any)"
        />

        <!-- Streaming content (replaces loading spinner) -->
        <div
          v-if="loading && hasStreamingContent"
          class="message-wrapper message-wrapper--assistant"
        >
          <div class="message-bubble message-bubble--assistant">
            <div class="message-role">Assistant</div>
            <div
              class="message-content markdown-body"
              v-html="renderMarkdown(streamingContent)"
            ></div>
          </div>
        </div>

        <!-- Fallback loading spinner when SSE hasn't started yet -->
        <div
          v-if="loading && !hasStreamingContent && isLastMessageUser"
          class="message-wrapper message-wrapper--assistant"
        >
          <div class="message-bubble message-bubble--assistant typing-indicator">
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
          <Icon name="send" size="md" />
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

.chat-header {
  @apply px-4 py-3 border-b border-gray-700 bg-gray-800/80 flex items-center justify-between backdrop-blur-sm z-10;
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

.btn-chat-close {
  @apply p-1.5 rounded-md hover:bg-gray-700 text-gray-500 hover:text-white transition-all;
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

.message-bubble--tool {
  @apply bg-gray-800 text-gray-200 border border-yellow-500/30 rounded-tl-sm;
}

.tool-result-details {
  @apply cursor-pointer outline-none;
}

.tool-result-summary {
  @apply flex items-center gap-2 select-none hover:opacity-80 transition-opacity outline-none list-none text-sm;
}

.tool-result-summary::-webkit-details-marker {
  display: none;
}

.tool-result-icon {
  @apply text-sm;
}

.tool-result-hint {
  @apply text-gray-600 text-[10px] italic;
}

.tool-result-body {
  @apply bg-[#161b22] border border-gray-800 rounded p-3 mt-2 text-[11px] text-green-500/80 overflow-y-auto max-h-80 whitespace-pre-wrap;
}

.tool-call-inline {
  @apply mt-2;
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
