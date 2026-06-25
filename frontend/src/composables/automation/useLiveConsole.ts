import { ref, computed } from "vue";
import { getMsgPayload } from "../../utils/dispatcher";
import { getPhaseMessage } from "../../constants/icons";
import type { AgentEvent, AgentMessagePayload, GuardrailBlockedPayload } from "../../types";
import { generateId } from "../../utils/crypto";
import { ApiService } from "../../services/api";

export function useLiveConsole(workspaceId: () => string, _isExecuting: () => boolean | undefined, historyEvents: () => AgentEvent[] | undefined) {
  const liveEvents = ref<AgentEvent[]>([]);
  const isConnected = ref(false);
  const pendingDecision = ref<GuardrailBlockedPayload | null>(null);
  const handledDecisionIds = new Set<string>();
  let eventSource: EventSource | null = null;

  const displayEvents = computed(() => {
    if (liveEvents.value.length > 0) return liveEvents.value;
    return historyEvents() || [];
  });

  const handleAgentEvent = (ev: AgentEvent) => {
    // Deduplicate against events already received (SSE reconnect replay).
    // Server-assigned IDs are stable across reconnections.
    if (ev.id && liveEvents.value.some(e => e.id === ev.id)) {
      return;
    }

    // Assign a unique ID if it doesn't have one for better Vue list diffing
    if (!ev.id) {
       (ev as any).id = generateId();
    }

    // Auto-clear old events if we detect a fresh automation start signal
    if (
      ev.type === "message" &&
      getMsgPayload(ev).content?.includes("▶ Booting automation:")
    ) {
      liveEvents.value = [];
      pendingDecision.value = null;
      return;
    }

    // Guardrail blocked — capture decision payload for approval UI
    // Skip if this decision was already handled (e.g. SSE replay on reconnect)
    if (ev.type === "guardrail_blocked") {
      const payload = ev.payload as GuardrailBlockedPayload;
      liveEvents.value.push(ev);
      if (!handledDecisionIds.has(payload.decision_id)) {
        pendingDecision.value = payload;
      }
      return;
    }

    // Guardrail invalidated — the decision was auto-resolved (e.g. automation
    // stopped).  Clear the pending prompt so the UI does not show a stale one.
    if (ev.type === "guardrail_invalidated") {
      const payload = ev.payload as { decision_id: string; reason: string };
      handledDecisionIds.add(payload.decision_id);
      if (pendingDecision.value?.decision_id === payload.decision_id) {
        pendingDecision.value = null;
      }
      liveEvents.value.push(ev);
      return;
    }
    // Handle streaming updates
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

    // Handle lifecycle events — append as system messages so they don't
    // overwrite the assistant's streaming content.
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

    // Handle final message events - Deduplicate against the immediately preceding streaming message
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

  const submitDecision = async (allow: boolean, persist: boolean) => {
    if (!pendingDecision.value) return;
    const id = pendingDecision.value.decision_id;
    try {
      await ApiService.submitGuardrailDecision(id, allow, persist);
    } catch (err) {
      console.error("Failed to submit guardrail decision", err);
    }
    handledDecisionIds.add(id);
    pendingDecision.value = null;
  };

  const connect = () => {
    if (eventSource) eventSource.close();

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
    pendingDecision.value = null;
  };

  const clearEvents = () => {
    liveEvents.value = [];
    pendingDecision.value = null;
  };

  return {
    liveEvents,
    displayEvents,
    isConnected,
    pendingDecision,
    connect,
    disconnect,
    clearEvents,
    submitDecision
  };
}
