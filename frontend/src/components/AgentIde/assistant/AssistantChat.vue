<script setup lang="ts">
import { ref, onMounted, watch, computed, nextTick } from "vue";
import { marked } from "marked";
import { useAssistant } from "../../../composables/useAssistant";
import { groupTurns } from "../../../utils/turnGrouper";
import { formatTime } from "../../../utils/time";
import GuardrailBanner from "../../../components/common/GuardrailBanner.vue";
import CollapsiblePanel from "../../../components/common/CollapsiblePanel.vue";
import UserMessage from "../../../components/common/UserMessage.vue";
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
  pendingDecision,
  thinking,
  liveReasoning,
  paused,
  fetchSessions,
  loadSession,
  newSession,
  sendMessage,
  deleteSession,
  activeWorkspaceId,
  cancel,
} = useAssistant();

const inputMessage = ref("");
const messageContainer = ref<HTMLElement | null>(null);
const sidebarCollapsed = ref(true);
const workCollapsed = ref<Record<number, boolean>>({});
const expandedSegments = ref<Set<string>>(new Set());
const isAtBottom = ref(true);

function segKey(turnIdx: number, segIdx: number): string {
  return `${turnIdx}-${segIdx}`;
}

function toggleSegment(turnIdx: number, segIdx: number) {
  const key = segKey(turnIdx, segIdx);
  if (expandedSegments.value.has(key)) {
    expandedSegments.value.delete(key);
  } else {
    expandedSegments.value.add(key);
  }
}

function isSegExpanded(turnIdx: number, segIdx: number): boolean {
  return expandedSegments.value.has(segKey(turnIdx, segIdx));
}

function toggleWork(turnIdx: number) {
  const current = !!workCollapsed.value[turnIdx];
  workCollapsed.value = {
    ...(workCollapsed.value as Record<number, boolean>),
    [turnIdx]: !current,
  };
}

function isWorkCollapsed(turnIdx: number): boolean {
  return !!workCollapsed.value[turnIdx];
}

function collapseAllWork() {
  const collapsed: Record<number, boolean> = {};
  turns.value.forEach((_, idx) => {
    collapsed[idx] = true;
  });
  workCollapsed.value = collapsed;
}

onMounted(() => {
  if (props.workspaceId) initWorkspace();
});

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

function onContainerScroll() {
  if (!messageContainer.value) return;
  const el = messageContainer.value;
  isAtBottom.value = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
}

function scrollToBottom() {
  if (!messageContainer.value) return;
  const el = messageContainer.value;
  if (el.scrollHeight - el.scrollTop - el.clientHeight > 80) return;
  el.scrollTop = el.scrollHeight;
}

function scrollSegmentIntoView(turnIdx: number, segIdx: number) {
  nextTick(() => {
    const el = document.querySelector(
      `[data-seg-key="${segKey(turnIdx, segIdx)}"]`
    );
    if (el && "scrollIntoView" in el) {
      (el as HTMLElement).scrollIntoView({ behavior: "instant", block: "nearest" });
    }
  });
}

const lastSegmentCount = ref(0);
watch(
  () => {
    const lastTurn = turns.value[turns.value.length - 1];
    return lastTurn?.segments.length ?? 0;
  },
  (newCount) => {
    if (newCount > lastSegmentCount.value) {
      const lastTurn = turns.value[turns.value.length - 1];
      if (lastTurn) {
        scrollSegmentIntoView(turns.value.length - 1, lastTurn.segments.length - 1);
      }
    }
    lastSegmentCount.value = newCount;
  }
);

watch(loading, async (newVal, oldVal) => {
  if (!oldVal && newVal) {
    await nextTick();
    const lastIdx = turns.value.length - 1;
    if (lastIdx >= 0) {
      workCollapsed.value = {
        ...(workCollapsed.value as Record<number, boolean>),
        [lastIdx]: false,
      };
    }
  }
  if (oldVal && !newVal) {
    await nextTick();
    collapseAllWork();
  }
});

const renderMd = (text: string) => marked.parse(text) as string;

const turns = computed(() => groupTurns(messages.value));
const lastMessageIsUser = computed(() => {
  if (messages.value.length === 0) return false;
  return messages.value[messages.value.length - 1]?.role === "user";
});

const formatTimestamp = () =>
  new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });

function toolLabel(name: string, args: string): string {
  if (!args || args === "{}") return name;
  let parsed: any;
  try {
    parsed = JSON.parse(args);
  } catch {
    return name;
  }
  const arg =
    parsed.path ||
    parsed.command ||
    parsed.query ||
    parsed.url ||
    parsed.summary ||
    "";
  if (typeof arg === "string" && arg.length > 40) {
    return `${name}  ${arg.slice(0, 40)}…`;
  }
  if (arg) return `${name}  ${arg}`;
  return name;
}

function toolIconClass(seg: { status: string }): string {
  if (seg.status === "running") return "tool-icon--running";
  if (seg.status === "error") return "tool-icon--error";
  return "tool-icon--success";
}
</script>

<template>
  <div class="assistant-shell">
    <CollapsiblePanel
      :collapsed="sidebarCollapsed"
      title="Conversations"
      position="left"
      @toggle="sidebarCollapsed = !sidebarCollapsed"
    >
      <template #header-actions>
        <button @click="handleNewChat" class="btn-new" title="New Chat">
          <Icon name="plus" size="sm" />
        </button>
      </template>
      <div class="session-list">
        <div v-if="sessions.length === 0" class="empty-sessions">
          No history in this workspace.
        </div>
        <div v-for="session in sessions" :key="session.id" class="session-row group">
          <button
            @click="handleLoadSession(session.id)"
            class="session-item"
            :class="{ 'session-item--active': currentSessionId === session.id }"
          >
            <div class="session-snippet">{{ session.snippet || "Empty conversation" }}</div>
            <div class="session-time">{{ formatTime(session.updated_at || "") }}</div>
          </button>
          <button @click.stop="handleDeleteSession(session.id)" class="btn-delete" title="Delete conversation">
            <Icon name="trash" size="xs" />
          </button>
        </div>
      </div>
    </CollapsiblePanel>

    <div class="chat-area">
      <header class="chat-header">
        <div class="flex items-center gap-4">
          <button
            v-if="sidebarCollapsed"
            @click="sidebarCollapsed = false"
            class="btn-sidebar-expand"
            title="Show Conversations"
          >
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

      <div v-if="error" class="chat-error">{{ error }}</div>
      <div v-if="pendingDecision" class="guardrail-banner-wrapper">
        <GuardrailBanner :decision="pendingDecision" @allow="(..._args: any[]) => {}" @deny="() => {}" />
      </div>

      <div class="message-container" ref="messageContainer" @scroll="onContainerScroll">
        <!-- Empty state -->
        <div v-if="messages.length === 0 && !loading" class="chat-empty">
          <div class="welcome-icon">💬</div>
          <h3>Workspace Assistant</h3>
          <p>You are talking to the agent bounded to <strong>{{ workspaceId }}</strong>.</p>
          <p>Ask it to scan files, check metrics, or help debug issues.</p>
        </div>

        <!-- Turns -->
        <template v-for="(turn, idx) in turns" :key="'turn-' + idx">
          <UserMessage
            :content="turn.userMessage"
            :timestamp="formatTimestamp()"
            @retry="handleRetry(turn.userMessage)"
          />

          <!-- Single assistant bubble: header + work + result -->
          <div
            v-if="turn.segments.length || turn.finalAnswer || (loading && idx === turns.length - 1)"
            :class="['message-wrapper', 'message-wrapper--assistant', { 'is-loading': loading && idx === turns.length - 1 }]"
          >
            <div class="message-bubble message-bubble--assistant">
              <button
                class="bubble-header"
                @click="toggleWork(idx)"
                :class="{ 'bubble-header--clickable': turn.segments.length > 0 }"
              >
                <span class="bubble-header-label">Assistant</span>
                <span v-if="turn.segments.length > 0" class="bubble-header-summary">
                  {{ turn.segments.filter(s => s.kind === 'tool_call').length }} step{{ turn.segments.filter(s => s.kind === 'tool_call').length !== 1 ? 's' : '' }} {{ loading && idx === turns.length - 1 ? 'in progress' : 'completed' }}
                </span>
                <span v-if="turn.segments.length > 0" class="bubble-header-chevron">
                  {{ isWorkCollapsed(idx) ? '▸' : '▾' }}
                </span>
              </button>

              <!-- Work section: reasoning + tool calls interleaved, live streaming reasoning at bottom -->
              <div v-if="(turn.segments.length > 0 || (loading && thinking && liveReasoning)) && !isWorkCollapsed(idx)" class="bubble-work-section">
                <div
                  v-for="(seg, sIdx) in turn.segments"
                  :key="`seg-${idx}-${sIdx}`"
                  :data-seg-key="segKey(idx, sIdx)"
                  class="segment-item"
                >
                  <!-- Reasoning text segment -->
                  <div v-if="seg.kind === 'reasoning'" class="bubble-reasoning markdown-body" v-html="renderMd(seg.text)"></div>

                  <!-- Tool call segment -->
                  <template v-if="seg.kind === 'tool_call'">
                    <button
                      class="segment-header"
                      :class="['segment-item--tool', toolIconClass(seg), { 'segment-item--running': seg.status === 'running' }]"
                      @click="toggleSegment(idx, sIdx)"
                    >
                      <span class="segment-icon">
                        <svg v-if="seg.status === 'running'" class="seg-spinner" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                          <path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83" />
                        </svg>
                        <svg v-else-if="seg.status === 'success'" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                          <polyline points="20 6 9 17 4 12" />
                        </svg>
                        <svg v-else width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                          <line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" />
                        </svg>
                      </span>
                      <span class="segment-label">
                        <span class="segment-name">{{ toolLabel(seg.name, seg.args) }}</span>
                      </span>
                      <span class="segment-chevron">{{ isSegExpanded(idx, sIdx) ? '▾' : '▸' }}</span>
                    </button>

                    <div v-if="isSegExpanded(idx, sIdx)" class="segment-detail">
                      <div class="segment-detail-row">
                        <span class="segment-detail-key">Args</span>
                        <pre class="segment-detail-value">{{ seg.args }}</pre>
                      </div>
                      <div v-if="seg.result" class="segment-detail-row">
                        <span class="segment-detail-key">Result</span>
                        <pre class="segment-detail-value">{{ seg.result }}</pre>
                      </div>
                      <div v-if="seg.error" class="segment-detail-row">
                        <span class="segment-detail-key">Error</span>
                        <pre class="segment-detail-value">{{ seg.error }}</pre>
                      </div>
                    </div>
                  </template>
                </div>
                <!-- Live reasoning text streaming during active thinking -->
                <div
                  v-if="loading && idx === turns.length - 1 && thinking && liveReasoning"
                  class="bubble-reasoning markdown-body"
                  v-html="renderMd(liveReasoning)"
                ></div>
              </div>

              <!-- Thinking dots during inactivity gaps -->
              <div
                :class="{ 'thinking-gap-hidden': !(loading && idx === turns.length - 1 && paused && !turn.finalAnswer) }"
                class="thinking-gap"
              >
                <span class="thinking-gap-dot"></span>
                <span class="thinking-gap-dot"></span>
                <span class="thinking-gap-dot"></span>
                <span class="thinking-gap-text">&nbsp;Thinking</span>
              </div>

              <!-- Final result section (always visible) -->
              <div v-if="turn.finalAnswer" class="bubble-result-section">
                <div class="bubble-result-label">Result</div>
                <div class="bubble-result-content markdown-body" v-html="renderMd(turn.finalAnswer)"></div>
              </div>
            </div>
          </div>
        </template>

        <!-- Interrupted response -->
        <div v-if="lastMessageIsUser && !loading" class="interrupted-bar">
          <Icon name="close" size="xs" />
          <span>Response interrupted — send a new message to continue</span>
        </div>
      </div>

      <div class="input-area">
        <textarea
          v-model="inputMessage"
          @keydown.enter.exact.prevent="handleSend"
          placeholder="Ask the workspace agent..."
          class="chat-input"
          :class="{ 'is-loading': loading }"
          rows="1"
          :disabled="loading"
        ></textarea>
        <button
          v-if="loading"
          @click="cancel"
          class="btn-stop"
          title="Stop"
        >
          <Icon name="close" size="md" />
        </button>
        <button
          v-else
          @click="handleSend"
          :disabled="!inputMessage.trim()"
          class="btn-send"
        >
          <Icon name="send" size="md" />
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
@import url('../../../styles/theme.css');

.assistant-shell {
  @apply h-full flex bg-gray-800 rounded-lg overflow-hidden border border-white/5;
}

.btn-new { @apply p-1.5 rounded-md hover:bg-gray-700 text-gray-400 hover:text-white transition-colors; }
.session-list { @apply flex-1 overflow-y-auto p-2 flex flex-col gap-1; }
.empty-sessions { @apply p-4 text-xs text-center text-gray-500 italic; }
.session-row { @apply relative flex items-center w-full px-2 overflow-hidden; }
.session-item { @apply flex-1 text-left p-3 pr-12 rounded-md transition-all flex flex-col gap-1 border border-transparent min-w-0 hover:bg-white/5; }
.session-item--active { @apply bg-blue-600/10 border-blue-500/30; }
.btn-delete { @apply absolute right-4 top-1/2 -translate-y-1/2 p-2 rounded-lg text-gray-500 opacity-0 transition-all scale-95 hover:bg-red-500/15 hover:text-red-400 flex items-center justify-center; }
.session-row:hover .btn-delete { @apply opacity-100 scale-100; }
.session-snippet { @apply text-sm text-gray-200 truncate font-medium block w-full; }
.session-time { @apply text-[10px] text-gray-500 font-mono; }

.chat-area { @apply flex-1 flex flex-col bg-gray-900 relative; }
.chat-error { @apply absolute top-0 left-0 right-0 z-10 bg-red-900/90 text-red-200 p-2 text-xs text-center font-medium; }
.chat-header { @apply px-4 py-3 border-b border-gray-700 bg-gray-800/80 flex items-center justify-between backdrop-blur-sm z-10; }
.chat-info { @apply flex flex-col; }
.chat-status { @apply text-[9px] font-bold text-green-500 uppercase tracking-widest leading-none mb-1; }
.chat-title { @apply text-xs font-bold text-gray-200 leading-none; }
.btn-sidebar-expand { @apply p-1.5 rounded-md bg-gray-800 hover:bg-gray-700 border border-gray-700 shadow-sm text-gray-400 hover:text-gray-200 transition-colors flex items-center justify-center focus:outline-none; }
.btn-chat-close { @apply p-1.5 rounded-md hover:bg-gray-700 text-gray-500 hover:text-white transition-all; }

.message-container { @apply flex-1 overflow-y-auto p-4 md:p-6 flex flex-col gap-5; }
.chat-empty { @apply m-auto flex flex-col items-center justify-center text-center text-gray-500 gap-3 max-w-sm px-6; }
.welcome-icon { @apply text-4xl mb-2 opacity-50; }
.message-wrapper { @apply flex w-full; }
.message-wrapper--assistant { @apply justify-start; }

.message-bubble {   @apply max-w-[85%] rounded-2xl p-4 flex flex-col gap-2 shadow-sm relative; }
.message-bubble--assistant { @apply bg-gray-800 text-gray-200 border border-gray-700 rounded-tl-sm; min-height: 60px; }
.message-bubble--assistant::before {
  content: "";
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 4px;
  background: linear-gradient(180deg, transparent 0%, transparent 15%, rgb(129, 140, 248) 50%, transparent 85%, transparent 100%);
  background-size: 100% 200%;
  background-repeat: no-repeat;
  opacity: 0;
  transition: opacity 200ms ease;
  border-radius: 2px;
}
.message-wrapper--assistant.is-loading .message-bubble--assistant::before {
  opacity: 1;
  animation: live-pulse 1.6s ease-in-out infinite;
  box-shadow: 0 0 8px rgba(129, 140, 248, 0.4);
}

@keyframes live-pulse {
  0% { background-position: 0% -100%; }
  100% { background-position: 0% 200%; }
}

.input-area {
  @apply p-4 border-t border-gray-700 bg-gray-800 flex gap-2 shrink-0;
}

.chat-input.is-loading {
  animation: input-glow 2s ease-in-out infinite;
}

@keyframes input-glow {
  0%   { box-shadow: 0 0 0 0px rgba(129, 140, 248, 0), 0 0 0px rgba(129, 140, 248, 0); }
  50%  { box-shadow: 0 0 0 1.5px rgba(129, 140, 248, 0.35), 0 0 10px rgba(129, 140, 248, 0.2); }
  100% { box-shadow: 0 0 0 0px rgba(129, 140, 248, 0), 0 0 0px rgba(129, 140, 248, 0); }
}

.bubble-header {
  @apply flex items-center gap-2 select-none w-full;
  background: none;
  border: none;
  padding: 0;
  text-align: left;
  font-family: inherit;
  color: inherit;
  font-size: inherit;
  cursor: pointer;
}
.bubble-header:hover .bubble-header-chevron { color: #d1d5db; }
.bubble-header-label {
  @apply text-[10px] font-bold uppercase tracking-wider text-gray-400;
}
.bubble-header-summary {
  @apply text-[10px] text-gray-500 font-normal flex-1 truncate;
}
.bubble-header-chevron {
  @apply text-gray-500 text-[10px] w-3 text-center;
}

.bubble-work-section {
  @apply flex flex-col gap-1 pt-1;
  min-height: 20px;
}

.bubble-reasoning {
  @apply text-xs leading-relaxed text-gray-300 px-1 pb-1;
}
.bubble-reasoning :deep(p) { @apply mb-2 last:mb-0; }
.bubble-reasoning :deep(ul) { @apply list-disc list-inside mt-1 mb-2; }
.bubble-reasoning :deep(ol) { @apply list-decimal list-inside mt-1 mb-2; }

.segment-item { @apply text-[11px]; }
.segment-header {
  @apply w-full flex items-center gap-2 px-1.5 py-1 rounded
         hover:bg-white/5 transition-colors text-left;
  border: none;
  cursor: pointer;
  font-family: inherit;
  font-size: inherit;
  color: inherit;
  background: transparent;
}
.segment-icon { @apply flex items-center justify-center w-3.5 h-3.5 shrink-0; }
.tool-icon--running .segment-icon,
.segment-item--running .segment-icon { @apply text-blue-400; }
.tool-icon--success .segment-icon,
.segment-item--tool:not(.segment-item--running):not(.tool-icon--error) .segment-icon { @apply text-green-500/70; }
.tool-icon--error .segment-icon,
.segment-item--tool.segment-item--running:not(.tool-icon--running) .segment-icon { @apply text-red-400/70; }
.segment-icon .seg-spinner { animation: live-pulse 1s linear infinite; transform-origin: center; }

.thinking-gap {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 4px 8px;
  font-size: 11px;
  color: rgb(107, 114, 128);
}
.thinking-gap-hidden {
  visibility: hidden;
}
.thinking-gap-dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: rgb(107, 114, 128);
  animation: thinking-pulse 1.2s ease-in-out infinite;
}
.thinking-gap-dot:nth-child(2) { animation-delay: 0.2s; }
.thinking-gap-dot:nth-child(3) { animation-delay: 0.4s; }
.thinking-gap-text {
  color: rgb(107, 114, 128);
  font-size: 11px;
}

@keyframes thinking-pulse {
  0%, 60%, 100% { opacity: 0.3; }
  30% { opacity: 1; }
}


@keyframes thinking-pulse {
  0%, 60%, 100% { opacity: 0.3; }
  30% { opacity: 1; }
}
.segment-label {
  @apply flex-1 font-mono text-gray-500 truncate flex items-center gap-2;
}
.segment-name { @apply shrink-0; }
.segment-chevron { @apply text-gray-600 text-[10px] w-3 text-center shrink-0; }

.segment-detail {
  @apply mt-1 ml-6 p-2 bg-gray-900/50 rounded border border-gray-700/40 flex flex-col gap-2;
}
.segment-detail-row { @apply flex flex-col gap-1; }
.segment-detail-key { @apply text-[10px] uppercase tracking-wider text-gray-500 font-semibold; }
.segment-detail-value {
  @apply text-xs font-mono text-gray-300 whitespace-pre-wrap break-words max-h-48 overflow-y-auto;
}

.bubble-result-section {
  @apply pt-2;
}
.bubble-result-label {
  @apply text-[10px] font-bold uppercase tracking-wider text-blue-400 mb-2;
}
.bubble-result-content { @apply text-sm leading-relaxed; }
.bubble-result-content :deep(p) { @apply mb-2 last:mb-0; }
.bubble-result-content :deep(pre) { @apply bg-gray-900 p-3 rounded mt-2 mb-2 overflow-x-auto text-xs; }
.bubble-result-content :deep(code) { @apply bg-gray-900 px-1 py-0.5 rounded text-xs text-blue-300; }
.bubble-result-content :deep(a) { @apply text-blue-400 hover:underline; }
.bubble-result-content :deep(ul) { @apply list-disc list-inside mt-1 mb-2; }
.bubble-result-content :deep(ol) { @apply list-decimal list-inside mt-1 mb-2; }

.bubble-empty-progress {
  display: flex;
  align-items: center;
  padding: 4px 0;
}

.bubble-empty-progress .thinking-text {
  font-size: 13px;
  color: var(--color-text-dim, #64748b);
  font-style: italic;
}

.interrupted-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 16px;
  background: rgba(239, 68, 68, 0.08);
  border: 1px solid rgba(239, 68, 68, 0.2);
  border-radius: 8px;
  font-size: 12px;
  color: var(--color-text-muted, #94a3b8);
  align-self: center;
}

.chat-input {
  @apply flex-1 bg-gray-900 border border-gray-700 rounded-xl px-4 py-3 text-sm text-gray-200 
         focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500 resize-none
         placeholder-gray-600 transition-all;
  position: relative;
  z-index: 1;
}
.chat-input:disabled { @apply opacity-50 cursor-not-allowed; }
.btn-send {
  @apply bg-blue-600 hover:bg-blue-500 text-white rounded-xl px-4 flex items-center justify-center
         transition-colors disabled:opacity-50 disabled:cursor-not-allowed shadow-md w-14 shrink-0;
}

.btn-stop {
  @apply bg-red-700 hover:bg-red-600 text-white rounded-xl px-4 flex items-center justify-center
         transition-colors shadow-md w-14 shrink-0;
}
</style>
