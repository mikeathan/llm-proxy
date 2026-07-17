import { ref } from "vue";
import { useSSEConnection } from "../network/useSSEConnection";
import type { AgentEvent, GuardrailBlockedPayload } from "../../types";
import { generateId } from "../../utils/crypto";

export interface SessionLifecyclePayload {
  phase: string
  conversation_id?: string
  workspace_id?: string
  snippet?: string
  source?: string
}

export function useAssistantSSE(
  workspaceId: () => string,
  onEvent?: (ev: AgentEvent) => void,
  onSessionUpdate?: (payload: SessionLifecyclePayload) => void,
) {
  const streamingContent = ref("");
  const liveEvents = ref<AgentEvent[]>([]);
  const pendingDecision = ref<GuardrailBlockedPayload | null>(null);
  const handledDecisionIds = new Set<string>();
  let receivedEventIds = new Set<string>();

  const handleAgentEvent = (ev: AgentEvent) => {
    // Server-side channel isolation already prevents automation events from
    // reaching this socket, but drop any non-assistant event defensively so a
    // misconfigured or legacy endpoint can't bleed automation output into chat.
    if (ev.channel && ev.channel !== "assistant") return;
    if (ev.id && receivedEventIds.has(ev.id)) return;
    if (ev.id) receivedEventIds.add(ev.id);
    if (!ev.id) (ev as any).id = generateId();

    if (ev.type === "lifecycle" && onSessionUpdate) {
      const p = ev.payload as any
      onSessionUpdate({
        phase: p?.phase ?? "",
        conversation_id: p?.conversation_id,
        workspace_id: p?.workspace_id,
        snippet: p?.snippet,
        source: p?.source,
      })
    }

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

    liveEvents.value.push(ev);
  };

  const sse = useSSEConnection({
    url: () =>
      `/admin/api/dispatcher/workspaces/${encodeURIComponent(workspaceId())}/live?channel=assistant`,
    onMessage: handleAgentEvent,
  });

  const disconnect = () => {
    sse.disconnect();
    pendingDecision.value = null;
    handledDecisionIds.clear();
  };

  const reset = () => {
    streamingContent.value = "";
    liveEvents.value = [];
    pendingDecision.value = null;
    receivedEventIds = new Set<string>();
    handledDecisionIds.clear();
  };

  return {
    streamingContent,
    liveEvents,
    isConnected: sse.isConnected,
    pendingDecision,
    connect: sse.connect,
    disconnect,
    reset,
  };
}
