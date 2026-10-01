<script setup lang="ts">
import { ref, watch, computed, nextTick, onMounted } from "vue";
import { useAssistant } from "../../../composables/assistant/useAssistant";
import { AssistantService } from "../../../services/assistant/assistantService";
import { groupTurns } from "../../../utils/message/turnGrouper";
import { useResponsiveLayout } from "../../../composables/ui/useResponsiveLayout";
import GuardrailBanner from "../../../components/common/chat/GuardrailBanner.vue";
import ConfirmDialog from "../../../components/ui/ConfirmDialog.vue";
import {
  MODEL_BUSY_CANCEL_LABEL,
  MODEL_BUSY_DIALOG_MESSAGE,
  MODEL_BUSY_DIALOG_TITLE,
  MODEL_BUSY_WAIT_LABEL,
} from "../../../constants/labels";
import ChatSessionList from "./ChatSessionList.vue";
import ChatMessages from "./ChatMessages.vue";
import ChatInput from "./ChatInput.vue";
import BaseButton from "../../common/buttons/BaseButton.vue";
import StatusTag from "../../common/display/StatusTag.vue";
import { useConfirm } from "../../../composables/ui/useConfirm";
import { usePinnedSessions } from "../../../composables/assistant/usePinnedSessions";
import type { StatusState } from "../../../types/ui";
import { useTurnInset } from "../../../composables/ui/useTurnInset";
import { useExpandedSegments } from "../../../composables/ui/useExpandedSegments";

const props = defineProps<{
  workspaceId: string;
  // The conversation addressed by the route (/workspaces/:ws/assistant/:id);
  // null for a new chat. The chat reports its own session changes back through
  // update:conversationId so the URL always names what is on screen.
  conversationId?: string | null;
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "update:conversationId", id: string | null): void;
}>();

// The session list becomes a slide-over below `sm`.
const { breakpoint } = useResponsiveLayout();
const isMobile = computed(() => breakpoint.value === "base");

const {
  loading, messages, sessions, currentSessionId, pendingDecision, submitDecision,
  thinking, liveReasoning, paused, phase, modelBusy, dismissModelBusy,
  fetchSessions, loadSession, newSession, sendMessage, deleteSession,
  deleteSessionsByIds, cancelSession, connectSSE, activeWorkspaceId, cancel,
  liveEvents, sseConnected,
} = useAssistant();
const { confirm } = useConfirm();
const { forget: forgetPins } = usePinnedSessions(() => props.workspaceId);

// The thin status strip (plan D14): what the assistant is doing, from the real
// run state — never a fixed "online".
const status = computed<{ state: StatusState; label: string }>(() => {
  if (pendingDecision.value) return { state: "running", label: "Waiting for your approval" };
  if (modelBusy.value) return { state: "queued", label: "Waiting for the model" };
  if (loading.value && !sseConnected.value) return { state: "error", label: "Reconnecting to the run" };
  if (loading.value) return { state: "running", label: "Working" };
  return { state: "neutral", label: "Ready" };
});

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


const turns = computed(() => {
  return groupTurns(messages.value)
})
const { insetCollapsed, isInsetCollapsed, toggleInset, collapseAllInsets, resetInsets } = useTurnInset(phase, turns);
const { expandedSegments, isSegExpanded, toggleSegment } = useExpandedSegments();

onMounted(() => { if (props.workspaceId) initWorkspace(); });
watch(() => props.workspaceId, () => initWorkspace());

const initWorkspace = async () => {
  activeWorkspaceId.value = props.workspaceId;
  newSession();
  await fetchSessions(props.workspaceId);
  connectSSE();
  if (props.conversationId) await handleLoadSession(props.conversationId);
};

// Route → chat: a deep link or back/forward to another conversation loads it.
watch(() => props.conversationId, (id) => {
  if (id && id !== currentSessionId.value) void handleLoadSession(id);
});
// Chat → route: a newly created session (first send) or one picked from the
// session list becomes the URL. Clearing is reported by the handlers that clear.
watch(currentSessionId, (id) => {
  if (id && id !== props.conversationId) emit("update:conversationId", id);
});
const forgetConversation = (removed: string[]) => {
  forgetPins(removed);
  if (props.conversationId && removed.includes(props.conversationId)) emit("update:conversationId", null);
};

const titleOf = (id: string) => sessions.value.find((s) => s.id === id)?.snippet || "this conversation";
const confirmDelete = (title: string, message: string) =>
  confirm({ title, message, type: "warning", confirmText: "Delete" });

  const handleNewChat = async () => {
    newSession();
    emit("update:conversationId", null);
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

// A busy local model means the run is waiting (see useMessageBuilder's
// modelBusy): offer Wait (keep waiting) or Cancel (abort the run). The dialog's
// close sets modelBusy to null via the v-model setter, which is the Wait action.
const showModelBusy = computed({
  get: () => modelBusy.value !== null,
  set: (value: boolean) => {
    if (!value) dismissModelBusy();
  },
});
const modelBusyMessage = computed(() => modelBusy.value ?? "");
const handleModelBusyCancel = () => {
  dismissModelBusy();
  void cancel();
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
  if (!(await confirmDelete(`Delete “${titleOf(sessionId)}”?`, "The conversation and its history are removed from this workspace."))) return;
  await deleteSession(props.workspaceId, sessionId);
  forgetConversation([sessionId]);
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
  if (!(await confirmDelete("Delete every conversation?", `All ${sessions.value.length} conversations in ${props.workspaceId} are removed. This cannot be undone.`))) return;
  try {
    const removed = sessions.value.map((s) => s.id);
    await AssistantService.deleteAllSessions(props.workspaceId);
    forgetConversation(removed);
    await fetchSessions(props.workspaceId);
  } catch (err) {
    console.error("Failed to clear all sessions", err);
  }
};

const handleDeleteGroup = async (ids: string[]) => {
  if (!(await confirmDelete(`Delete ${ids.length} conversations?`, "Every conversation in this group is removed. This cannot be undone."))) return;
  await deleteSessionsByIds(props.workspaceId, ids);
  forgetConversation(ids);
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
        :workspace-id="workspaceId"
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
          :workspace-id="workspaceId"
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
        <div class="flex min-w-0 items-center gap-2">
          <span class="relative">
            <BaseButton
              variant="ghost"
              size="sm"
              :icon="sidebarOpen ? 'chevron-left' : 'chevron-right'"
              icon-only
              :label="sidebarOpen ? 'Hide conversations' : 'Show conversations'"
              :aria-expanded="sidebarOpen"
              @click="toggleSidebar"
            />
            <span v-if="inboundCount > 0 && !sidebarOpen" class="badge-dot" aria-hidden="true" />
          </span>
          <BaseButton variant="ghost" size="sm" icon="plus" icon-only label="New chat" @click="handleNewChat" />
          <h2 class="m-0 min-w-0 truncate font-mono text-[length:var(--text-small)] font-medium text-primary">{{ workspaceId }}</h2>
          <StatusTag data-test="chat-status" :state="status.state" :label="status.label" />
        </div>
        <BaseButton variant="ghost" size="sm" icon="close" icon-only label="Close the assistant" @click="emit('close')" />
      </header>

      <div v-if="pendingDecision" class="guardrail-banner-wrapper">
        <GuardrailBanner :decision="pendingDecision" :submit="submitDecision" />
      </div>

      <p v-if="messages.length === 0 && currentSessionRunning && !loading" role="status" class="chat-processing-banner">
        This conversation is running — its output appears as it arrives.
      </p>

      <ChatMessages
        ref="chatMessagesRef"
        :messages="messages"
        :turns="turns"
        :loading="loading"
        :thinking="thinking"
        :live-reasoning="liveReasoning"
        :paused="paused"
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

    <ConfirmDialog
      v-model="showModelBusy"
      :title="MODEL_BUSY_DIALOG_TITLE"
      :message="modelBusyMessage || MODEL_BUSY_DIALOG_MESSAGE"
      type="warning"
      :confirm-text="MODEL_BUSY_WAIT_LABEL"
      :cancel-text="MODEL_BUSY_CANCEL_LABEL"
      @confirm="dismissModelBusy"
      @cancel="handleModelBusyCancel"
    />
  </div>
</template>

<style scoped lang="postcss">
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
  @apply fixed inset-0 bg-scrim/50 z-30;
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
  @apply flex-1 flex flex-col bg-canvas relative min-w-0;
}

.chat-header {
  @apply flex min-h-11 items-center justify-between gap-2 border-b border-hairline bg-surface px-3 py-1.5 z-10;
}

.badge-dot {
  @apply absolute -top-0.5 -right-0.5 h-2 w-2 rounded-full bg-state-error;
}

.guardrail-banner-wrapper {
  @apply px-4 py-2;
}

.chat-processing-banner {
  @apply m-0 px-6 py-3 text-[length:var(--text-small)] text-muted;
}
</style>
