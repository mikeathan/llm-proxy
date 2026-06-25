import { ref } from "vue";
import type { AgentEvent, GuardrailBlockedPayload } from "../types";
import { generateId } from "../utils/crypto";

export function useAssistantSSE(
  workspaceId: () => string,
  onEvent?: (ev: AgentEvent) => void,
) {
  const streamingContent = ref("");
  const liveEvents = ref<AgentEvent[]>([]);
  const isConnected = ref(false);
  const pendingDecision = ref<GuardrailBlockedPayload | null>(null);
  const handledDecisionIds = new Set<string>();
  let eventSource: EventSource | null = null;
  let receivedEventIds = new Set<string>();

  const handleAgentEvent = (ev: AgentEvent) => {
    if (ev.id && receivedEventIds.has(ev.id)) return;
    if (ev.id) receivedEventIds.add(ev.id);
    if (!ev.id) (ev as any).id = generateId();

    // Call external handler first (for messageBuilder, guardrails, etc.)
    onEvent?.(ev);

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
      streamingContent.value = ev.payload as string;
      liveEvents.value.push(ev);
      return;
    }

    if (ev.type === "lifecycle") {
      liveEvents.value.push(ev);
      return;
    }

    if (ev.type === "tool_call" || ev.type === "tool_result" || ev.type === "step_start") {
      liveEvents.value.push(ev);
      return;
    }

    if (ev.type === "message") {
      liveEvents.value.push(ev);
      return;
    }

    liveEvents.value.push(ev);
  };

  const connect = () => {
    if (eventSource) eventSource.close();

    isConnected.value = false;

    const url = `/admin/api/dispatcher/workspaces/${workspaceId()}/live`;
    eventSource = new EventSource(url);

    eventSource.addEventListener("ping", () => {
      isConnected.value = true;
    });

    eventSource.addEventListener("agent_update", (e) => {
      try {
        const ev = JSON.parse(e.data) as AgentEvent;
        handleAgentEvent(ev);
      } catch (err) {
        console.error("Failed to parse agent event", err);
      }
    });

    eventSource.onerror = () => {
      isConnected.value = false;
    };
  };

  const disconnect = () => {
    if (eventSource) {
      eventSource.close();
      eventSource = null;
    }
    isConnected.value = false;
    pendingDecision.value = null;
  };

  const reset = () => {
    streamingContent.value = "";
    liveEvents.value = [];
    pendingDecision.value = null;
    receivedEventIds = new Set<string>();
  };

  return {
    streamingContent,
    liveEvents,
    isConnected,
    pendingDecision,
    connect,
    disconnect,
    reset,
  };
}
