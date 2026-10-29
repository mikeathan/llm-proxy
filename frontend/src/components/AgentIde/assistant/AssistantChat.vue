<script setup lang="ts">
import { ref, watch, computed, nextTick, onMounted, onBeforeUnmount, toRef } from "vue";
import { useAssistant } from "../../../composables/assistant/useAssistant";
import { useMessageQueue } from "../../../composables/assistant/useMessageQueue";
import { peekDraft, stashDraft } from "../../../composables/assistant/useComposerDraft";
import { useMemoryReview } from "../../../composables/memory/useMemoryReview";
import { useToast } from "../../../composables/useToast";
import MemoryReviewDialog from "../memory/MemoryReviewDialog.vue";
import { AssistantService } from "../../../services/assistant/assistantService";
import { CommandService } from "../../../services/commands/commandService";
import type { CommandTurn } from "../../../types/commands";
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
import { useProfiles } from "../../../composables/assistant/useProfiles";
import { useHelpers } from "../../../composables/assistant/useHelpers";
import { useWorkspaceProfile } from "../../../composables/assistant/useWorkspaceProfile";
import { defaultProfile, profileOptions } from "../../../utils/assistant/profileChoice";
import RunSettings from "./RunSettings.vue";
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
  loading, runToken, runEnd, messages, sessions, currentSessionId, pendingDecision, submitDecision,
  thinking, liveReasoning, paused, phase, modelBusy, dismissModelBusy,
  fetchSessions, loadSession, newSession, openWorkspace, sendMessage, deleteSession,
  deleteSessionsByIds, cancelSession, cancel,
  liveEvents, sseConnected, conversationProfile, helpersOn,
} = useAssistant();
// What the next message runs with. The model is remembered per browser (empty = the primary). The profile belongs to
// the conversation: a new chat starts with the workspace's (else the default for chats) until one is picked here, and
// nothing is sent while none was, so a conversation keeps the profile it already has.
const { model, options: modelOptions } = useAssistantModel();
const { profiles } = useProfiles();
const { profile: workspaceProfile } = useWorkspaceProfile(toRef(props, "workspaceId"));
const startProfile = computed(() => defaultProfile(profiles.value, "assistant", workspaceProfile.value));
const profileChoices = computed(() => profileOptions(profiles.value, "assistant", workspaceProfile.value));
// A conversation whose profile no longer exists (a deleted file) shows the starting one, so the control stays usable.
const shownProfile = computed(() => profiles.value.find((p) => p.id === conversationProfile.value) ?? startProfile.value);
// Only a profile the catalog knows is sent: a conversation that names a deleted one is not refused (the server skips
// an id it no longer has and applies the next level).
const profileToSend = computed(() => (profiles.value.some((p) => p.id === conversationProfile.value) ? conversationProfile.value : ""));
const profileChoice = computed({
  get: () => shownProfile.value?.id ?? "",
  set: (id: string) => { conversationProfile.value = id; },
});
// "Use helpers": offered for a profile that names helpers, and asked of the server only when the switch is on and the
// server says helpers can run (a cloud helper model is set up). The switch lasts for the open conversation.
const { status: helpersStatus, refresh: refreshHelpers } = useHelpers();
const helpersShown = computed(() => (shownProfile.value?.delegates?.length ?? 0) > 0);
const helpersToSend = computed(() => helpersOn.value && helpersShown.value && helpersStatus.value.available);
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

// What was in the box when the chat was last taken down (a 401 and a trip to the sign-in page) comes back with it.
const inputMessage = ref(peekDraft(props.workspaceId));
onBeforeUnmount(() => stashDraft(props.workspaceId, inputMessage.value));
// Text given back to the box (a queued message that could not be sent, a refused command); a draft already in the box
// is kept, with the returned text after it.
const restoreDraft = (text: string) => {
  inputMessage.value = inputMessage.value.trim() ? `${inputMessage.value}\n\n${text}` : text;
};
// A message written while a run is going waits behind it (useMessageQueue) and goes back to the box as a draft when it
// cannot be sent.
const messageQueue = useMessageQueue({
  runToken, runEnd, loading,
  conversation: currentSessionId,
  workspaceId: () => props.workspaceId,
  send: (text) => submitText(text),
  restoreDraft,
});
const queuedText = computed(() => messageQueue.queued.value?.text ?? null);
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
// output after the DOM updates. A turn command's turn brings its own agent and
// helpers, for this message only; the model and the conversation's profile stay.
async function launchRun(text: string, turn?: CommandTurn) {
  collapseAllInsets();
  const helpers = turn ? turn.use_helpers === true : helpersToSend.value;
  await sendMessage(props.workspaceId, text, {
    model: model.value, profile: profileToSend.value, ...(turn?.agent ? { agent: turn.agent } : {}), ...(helpers && { useHelpers: true }),
  });
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

onMounted(() => {
  void refreshHelpers(); // the Helper model may have been changed in Settings since the chat was last open
  messageQueue.reclaim();
  if (props.workspaceId) void initWorkspace().then(() => messageQueue.reclaim());
});
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
  // /word) goes to the assistant as usual; a refused command keeps its text so it can be corrected. A turn command
  // (/research, /agent) answers with the turn to send instead, after its note when it has one.
  type CommandOutcome = { kind: "done" | "refused" | "none" } | { kind: "turn"; turn: CommandTurn };
  const runCommand = async (text: string): Promise<CommandOutcome> => {
    let result;
    try {
      result = await CommandService.run(text, props.workspaceId);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "The command could not be run");
      return { kind: "refused" };
    }
    if (!result.handled) return { kind: "none" };
    if (result.failed) {
      toast.error(result.reply);
      return { kind: "refused" };
    }
    if (result.turn) {
      if (result.reply) toast.warn(result.reply);
      return { kind: "turn", turn: result.turn };
    }
    toast.success(result.reply);
    return { kind: "done" };
  };

  // The one way a written message is acted on: a command is answered by the backend, anything else starts a run. A
  // refused command is given back to the box so it can be corrected.
  async function submitText(text: string) {
    const command: CommandOutcome = text.startsWith("/") ? await runCommand(text) : { kind: "none" };
    if (command.kind === "refused") restoreDraft(text);
    else if (command.kind === "turn") await launchRun(command.turn.message, command.turn);
    else if (command.kind === "none") await launchRun(text);
  }

  const handleSend = async () => {
    const text = inputMessage.value.trim();
    if (!text) return;
    if (loading.value) {
      await queueBehindRun(text);
      return;
    }
    inputMessage.value = "";
    await submitText(text);
  };

  // Sending would replace the live run, so the message is queued behind it. One message waits: a second asks first.
  const queueBehindRun = async (text: string) => {
    if (queuedText.value !== null && !(await confirm({ title: "Replace the queued message?", message: `“${queuedText.value}” has not been sent yet.`, type: "warning", confirmText: "Replace" }))) return;
    if (messageQueue.enqueue(text)) {
      inputMessage.value = "";
      return;
    }
    // The run ended while asking: there is nothing to wait behind.
    inputMessage.value = "";
    await submitText(text);
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
        :queued="queuedText"
        :workspace-id="workspaceId"
        @send="handleSend"
        @unqueue="messageQueue.remove()"
        @cancel="cancel"
        @review="startReview()"
        @update:input-message="inputMessage = $event"
      >
        <template #tools>
          <RunSettings
            v-if="shownProfile"
            v-model:profile="profileChoice"
            v-model:model="model"
            v-model:helpers="helpersOn"
            :helpers-shown="helpersShown"
            :helpers-available="helpersStatus.available"
            :helpers-model="helpersStatus.model"
            :helpers-problem="helpersStatus.problem"
            :profile-options="profileChoices"
            :model-options="modelOptions"
            :profile-name="shownProfile.name"
            :profile-summary="shownProfile.summary"
            :disabled="loading"
            :sheet="isMobile"
          />
        </template>
      </ChatInput>
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
