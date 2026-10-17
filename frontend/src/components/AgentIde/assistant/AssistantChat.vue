<script setup lang="ts">
import { ref, watch, computed, nextTick, onMounted, toRef } from "vue";
import { useAssistant } from "../../../composables/assistant/useAssistant";
import { useMemoryReview } from "../../../composables/memory/useMemoryReview";
import { useToast } from "../../../composables/useToast";
import MemoryReviewDialog from "../memory/MemoryReviewDialog.vue";
import { AssistantService } from "../../../services/assistant/assistantService";
import { CommandService } from "../../../services/commands/commandService";
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
import { useAssistantModel } from "../../../composables/assistant/useAssistantModel";
import BaseButton from "../../common/buttons/BaseButton.vue";
import StatusTag from "../../common/display/StatusTag.vue";
import { useConfirm } from "../../../composables/ui/useConfirm";
import { usePinnedSessions } from "../../../composables/assistant/usePinnedSessions";
import type { StatusState } from "../../../types/ui";
import { useTurnInset } from "../../../composables/ui/useTurnInset";
import { usePersistedState } from "../../../composables/ui/usePersistedState";
import { useExpandedSegments } from "../../../composables/ui/useExpandedSegments";

const props = defineProps<{
  workspaceId: string;
  // The conversation addressed by the route (/workspaces/:ws/assistant/:id);
  // null for a new chat. The chat reports its own session changes back through
  // update:conversationId so the URL always names what is on screen.
  conversationId?: string | null;
  // `panel` is the narrow ask panel on any page: the conversation list is always
  // an overlay inside it (never a column) and nothing is persisted for it.
  variant?: "page" | "panel";
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "update:conversationId", id: string | null): void;
}>();

// The session list becomes a slide-over below `sm`.
const { breakpoint } = useResponsiveLayout();
const isMobile = computed(() => breakpoint.value === "base");
// Where the conversation list is an overlay and the header is icon-only: a phone, or the narrow panel.
const compact = computed(() => props.variant === "panel" || isMobile.value);

const {
  loading, messages, sessions, currentSessionId, pendingDecision, submitDecision,
  thinking, liveReasoning, paused, phase, modelBusy, dismissModelBusy,
  fetchSessions, loadSession, newSession, openWorkspace, sendMessage, deleteSession,
  deleteSessionsByIds, cancelSession, cancel,
  liveEvents, sseConnected,
} = useAssistant();
// The model the operator picked for the next message (empty = the default, the primary).
const { model, options: modelOptions } = useAssistantModel();
const { confirm } = useConfirm();
const { forget: forgetPins } = usePinnedSessions(() => props.workspaceId);

// "Review for memories": the model proposes facts from this conversation, the operator ticks what to keep.
const toast = useToast();
const {
  open: reviewOpen, loading: reviewLoading, saving: reviewSaving, error: reviewError, items: reviewItems,
  selectedCount: reviewSelected, start: startReview, toggle: toggleReviewItem, save: saveReview, close: closeReview,
} = useMemoryReview(toRef(props, "workspaceId"), currentSessionId);
const handleSaveReview = async () => {
  const saved = await saveReview();
  if (saved > 0) toast.success(saved === 1 ? "Saved 1 memory" : `Saved ${saved} memories`);
};

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
// Whether the conversation list is open is a per-browser preference on a roomy
// screen (closed until the operator opens it); on a phone it is a drawer that always
// starts closed.
const HISTORY_OPEN_KEY = "assistant-history-open";
const UNTITLED_CONVERSATION = "Conversation";
const historyOpenPref = usePersistedState<boolean>(HISTORY_OPEN_KEY, {
  version: 1,
  fallback: false,
  parse: (data) => (typeof data === "boolean" ? data : null),
});
const drawerOpen = ref(false);
const sidebarOpen = computed({
  get: () => (compact.value ? drawerOpen.value : historyOpenPref.value),
  set: (open: boolean) => {
    if (compact.value) drawerOpen.value = open;
    else historyOpenPref.value = open;
  },
});
// A conversation that has not started: there is nothing to start new from, so New is not offered.
const isFreshConversation = computed(() => !currentSessionId.value && messages.value.length === 0);
const conversationTitle = computed(() => sessions.value.find((s) => s.id === currentSessionId.value)?.snippet || UNTITLED_CONVERSATION);
const inboundCount = ref(0);
const chatMessagesRef = ref<InstanceType<typeof ChatMessages> | null>(null);
const chatInputRef = ref<InstanceType<typeof ChatInput> | null>(null);
// The ask panel puts the cursor in the message when it opens.
defineExpose({ focusInput: () => chatInputRef.value?.focus() });

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
  await sendMessage(props.workspaceId, text, model.value);
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
  resetInsets();
  await openWorkspace(props.workspaceId, props.conversationId ?? null);
  if (props.conversationId) await settleLoadedSession();
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

  // A slash command is answered by the backend without the assistant. Text that is not a command (or an unknown
  // /word) goes to the assistant as usual; a refused command keeps its text so it can be corrected.
  const runCommand = async (text: string): Promise<boolean> => {
    let result;
    try {
      result = await CommandService.run(text);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "The command could not be run");
      return true;
    }
    if (!result.handled) return false;
    if (result.failed) toast.error(result.reply);
    else {
      toast.success(result.reply);
      inputMessage.value = "";
    }
    return true;
  };

  const handleSend = async () => {
    const text = inputMessage.value.trim();
    if (!text || loading.value) return;
    if (text.startsWith("/") && await runCommand(text)) return;
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

  // The view settles after a conversation is loaded: finished turns collapse,
  // the latest is in view, and the mobile sidebar gets out of the way.
  const settleLoadedSession = async () => {
    collapseAllInsets();
    await scrollToLatest();
    if (compact.value) {
      sidebarOpen.value = false;
    }
  };

  const handleLoadSession = async (sessionId: string) => {
    resetInsets();
    await loadSession(props.workspaceId, sessionId);
    await settleLoadedSession();
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
      v-if="!compact"
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
      <div v-if="compact && sidebarOpen" class="mobile-drawer" @click.stop>
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
        v-if="compact && sidebarOpen"
        class="mobile-backdrop"
        @click="toggleSidebar"
      />
    </Transition>

    <div class="chat-area">
      <header class="chat-header">
        <div class="flex min-w-0 items-center gap-2">
          <!-- History and New are one control, so New is never alone; with the list open on a roomy screen the list holds both. -->
          <div v-if="compact || !sidebarOpen" class="header-cap" role="group" aria-label="Conversations">
            <span class="relative">
              <BaseButton
                variant="ghost"
                size="sm"
                :icon="compact ? 'menu' : 'chevron-right'"
                icon-only
                :label="sidebarOpen ? 'Hide conversations' : 'Show conversations'"
                :aria-expanded="sidebarOpen"
                @click="toggleSidebar"
              />
              <span v-if="inboundCount > 0 && !sidebarOpen" class="badge-dot" aria-hidden="true" />
            </span>
            <span v-if="!isFreshConversation" class="cap-sep" aria-hidden="true"></span>
            <BaseButton v-if="!isFreshConversation" data-test="header-new" variant="ghost" size="sm" icon="plus" :icon-only="compact" label="New chat" aria-label="New chat" class-name="!text-primary" @click="handleNewChat">New</BaseButton>
          </div>
          <!-- A ready chat shows nothing (assistive technology still hears "Ready"); the state leads the title only while the chat is doing something. -->
          <span v-if="status.state === 'neutral'" data-test="chat-status" class="sr-only" role="status">{{ status.label }}</span>
          <StatusTag v-else data-test="chat-status" :state="status.state" :label="status.label" class="flex-none" />
          <h2 v-if="!isFreshConversation" class="m-0 min-w-0 truncate font-mono text-[length:var(--text-small)] font-medium text-primary">{{ conversationTitle }}</h2>
        </div>
        <div class="flex flex-none items-center gap-1">
          <BaseButton variant="ghost" size="sm" icon="close" icon-only label="Close the assistant" @click="emit('close')" />
        </div>
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
        ref="chatInputRef"
        :compact="isMobile"
        :can-review="!!currentSessionId"
        :loading="loading"
        :paused="paused"
        :input-message="inputMessage"
        v-model:model="model"
        :model-options="modelOptions"
        @send="handleSend"
        @cancel="cancel"
        @review="startReview()"
        @update:input-message="inputMessage = $event"
      />
    </div>

    <MemoryReviewDialog
      :open="reviewOpen"
      :loading="reviewLoading"
      :saving="reviewSaving"
      :error="reviewError"
      :items="reviewItems"
      :selected-count="reviewSelected"
      @update:open="(open: boolean) => { if (!open) closeReview(); }"
      @toggle="toggleReviewItem"
      @save="handleSaveReview"
      @retry="startReview()"
    />

    <!-- Teleported with the other overlays opened from here, so its scrim covers the whole screen. -->
    <Teleport to="body">
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
    </Teleport>
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
  @apply absolute inset-0 bg-scrim/50 z-30;
}

/* The list slides over the chat box (a phone's, or the ask panel's), never over the top strip or the tab bar. */
.mobile-drawer {
  @apply absolute left-0 top-0 bottom-0 w-[85%] max-w-[320px] z-40;
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
  @apply flex min-h-11 items-center justify-between gap-2 bg-canvas px-3 py-1.5 z-10;
}

.header-cap {
  @apply inline-flex flex-none items-center rounded-[var(--radius-md)] bg-surface-raised p-0.5;
}
.cap-sep {
  @apply h-4 w-px flex-none bg-border-strong;
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
