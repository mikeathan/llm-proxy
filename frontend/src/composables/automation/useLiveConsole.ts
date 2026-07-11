import { ref, computed } from "vue";
import { useSSEConnection } from "../network/useSSEConnection";
import { getMsgPayload } from "../../utils/dispatcher";
import { getPhaseMessage } from "../../constants/icons";
import type { AgentEvent, AgentMessagePayload, GuardrailBlockedPayload } from "../../types";
import { generateId } from "../../utils/crypto";
import { post } from "../../services/httpClient";

export function useLiveConsole(workspaceId: () => string, _isExecuting: () => boolean | undefined, historyEvents: () => AgentEvent[] | undefined) {
  const liveEvents = ref<AgentEvent[]>([]);
  const pendingDecision = ref<GuardrailBlockedPayload | null>(null);
  const handledDecisionIds = new Set<string>();

  const displayEvents = computed(() => {
    if (liveEvents.value.length > 0) return liveEvents.value;
    return historyEvents() || [];
  });

  const handleAgentEvent = (ev: AgentEvent) => {
    if (ev.id && liveEvents.value.some(e => e.id === ev.id)) return;
    if (!ev.id) {
       (ev as any).id = generateId();
    }
    if (
      ev.type === "message" &&
      getMsgPayload(ev).content?.includes("▶ Booting automation:")
    ) {
      liveEvents.value = [];
      pendingDecision.value = null;
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
    if (ev.type === "tool_stream") {
      const content = ev.payload as string;
      const lastIdx = liveEvents.value.length - 1;
      const lastEvent = lastIdx >= 0 ? liveEvents.value[lastIdx] : null;
      if (
        lastEvent &&
        lastEvent.type === "message" &&
        (lastEvent.payload as AgentMessagePayload).role === "assistant"
      ) {
        (lastEvent.payload as AgentMessagePayload).content = content;
      } else {
        liveEvents.value.push({
          type: "message",
          timestamp: ev.timestamp,
          payload: {
            role: "assistant",
            content: content,
          } as AgentMessagePayload,
        });
      }
      return;
    }
    if (ev.type === "reasoning") {
      const content = ev.payload as string;
      const lastIdx = liveEvents.value.length - 1;
      const lastEvent = lastIdx >= 0 ? liveEvents.value[lastIdx] : null;
      if (
        lastEvent &&
        lastEvent.type === "message" &&
        (lastEvent.payload as AgentMessagePayload).role === "assistant"
      ) {
        (lastEvent.payload as AgentMessagePayload).content = content;
      } else {
        liveEvents.value.push({
          type: "message",
          timestamp: ev.timestamp,
          payload: {
            role: "assistant",
            content: content,
          } as AgentMessagePayload,
        });
      }
      return;
    }
    if (ev.type === "lifecycle") {
      const payload = ev.payload as Record<string, any>;
      const text = getPhaseMessage(payload.phase as string, payload);
      if (text) {
        liveEvents.value.push({
          type: "message",
          timestamp: ev.timestamp,
          payload: {
            role: "system",
            content: text,
          } as AgentMessagePayload,
        });
      }
      return;
    }
    if (ev.type === "message") {
      const payload = getMsgPayload(ev);
      if (payload.role === "assistant") {
        const lastEvent = liveEvents.value[liveEvents.value.length - 1];
        if (
          lastEvent &&
          lastEvent.type === "message" &&
          (lastEvent.payload as AgentMessagePayload).role === "assistant"
        ) {
          if (!payload.content && (lastEvent.payload as AgentMessagePayload).content) {
            return;
          }
          liveEvents.value[liveEvents.value.length - 1] = ev;
          return;
        }
      }
    }
    liveEvents.value.push(ev);
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

  const clearEvents = () => {
    liveEvents.value = [];
    pendingDecision.value = null;
    handledDecisionIds.clear();
  };

  return {
    liveEvents,
    displayEvents,
    isConnected: sse.isConnected,
    pendingDecision,
    connect: sse.connect,
    disconnect,
    clearEvents,
    submitDecision,
  };
}
