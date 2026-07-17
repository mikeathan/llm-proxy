import { ref, computed, watch } from "vue";
import { useSSEConnection } from "../network/useSSEConnection";
import { getMsgPayload } from "../../utils/dispatcher";
import type { AgentEvent, GuardrailBlockedPayload } from "../../types";
import { generateId } from "../../utils/crypto";
import { post } from "../../services/httpClient";
import { useMessageBuilder } from "../../utils/message/messageBuilder";
import type { AssistantMessage } from "../../types/assistant";

export function useLiveConsole(workspaceId: () => string, _isExecuting: () => boolean | undefined, historyEvents: () => AgentEvent[] | undefined, runName?: string) {
  const liveEvents = ref<AgentEvent[]>([]);
  const pendingDecision = ref<GuardrailBlockedPayload | null>(null);
  const handledDecisionIds = new Set<string>();

  const messages = ref<AssistantMessage[]>([]);
  const builder = useMessageBuilder(messages, {
    source: 'automation',
    finalizeOn: 'lifecycle',
    headerMessage: { role: 'user', content: runName ? `Automation run: ${runName}` : 'Automation run' },
  });

  const displayEvents = computed(() => {
    if (liveEvents.value.length > 0) return liveEvents.value;
    return historyEvents() || [];
  });

  // displayMessages IS the builder's live messages ref (single source of truth).
  const displayMessages = messages;

  const handleAgentEvent = (ev: AgentEvent) => {
    if (ev.id && liveEvents.value.some(e => e.id === ev.id)) return;
    if (!ev.id) {
       (ev as any).id = generateId();
    }
    if (
      ev.type === "message" &&
      getMsgPayload(ev).content?.includes("▶ Booting automation:")
    ) {
      resetRun();
      return;
    }
    if (ev.type === "guardrail_blocked") {
      const payload = ev.payload as GuardrailBlockedPayload;
      liveEvents.value.push(ev);
      if (!handledDecisionIds.has(payload.decision_id)) {
        pendingDecision.value = payload;
      }
      return;
    }
    if (ev.type === "guardrail_invalidated") {
      const payload = ev.payload as { decision_id: string; reason: string };
      handledDecisionIds.add(payload.decision_id);
      if (pendingDecision.value?.decision_id === payload.decision_id) {
        pendingDecision.value = null;
      }
      liveEvents.value.push(ev);
      return;
    }
    // All other events flow through the shared builder (single consumption path).
    builder.handleEvent(ev);
  };

  const sse = useSSEConnection({
    url: () => `/admin/api/dispatcher/workspaces/${workspaceId()}/live`,
    onMessage: handleAgentEvent,
  });

  const submitDecision = async (allow: boolean, persist: boolean) => {
    if (!pendingDecision.value) return;
    const id = pendingDecision.value.decision_id;
    try {
      await post('/admin/api/conversation/guardrail-decision', { decision_id: id, allow, persist });
    } catch (err) {
      console.error("Failed to submit guardrail decision", err);
    }
    handledDecisionIds.add(id);
    pendingDecision.value = null;
  };

  const disconnect = () => {
    sse.disconnect();
    pendingDecision.value = null;
    handledDecisionIds.clear();
  };

  function resetRun() {
    builder.reset();
    pendingDecision.value = null;
    handledDecisionIds.clear();
    messages.value = [{
      role: 'user',
      content: runName ? `Automation run: ${runName}` : 'Automation run',
    }];
  }

  // Run-end fallback: if the live run ends without a lifecycle{completed}
  // (interrupted), finalize with the last streamed answer so it is not dropped.
  let wasExecuting = false;
  watch(
    () => _isExecuting(),
    (executing) => {
      if (wasExecuting && !executing) {
        const last = [...messages.value].reverse().find(m => m.role === 'assistant' && m.content);
        if (last) builder.finalize(last.content);
      }
      wasExecuting = !!executing;
    },
    { immediate: true },
  );

  const connectWithReset = () => {
    resetRun();
    for (const ev of (historyEvents() || [])) builder.handleEvent(ev);
    sse.connect();
  };

  const clearEvents = () => {
    resetRun();
    sse.reset();
  };

  return {
    liveEvents,
    displayEvents,
    displayMessages,
    thinking: builder.thinking,
    liveReasoning: builder.liveReasoning,
    paused: builder.paused,
    phase: builder.phase,
    isConnected: sse.isConnected,
    pendingDecision,
    connect: connectWithReset,
    disconnect,
    clearEvents,
    submitDecision,
  };
}
