import { ref, computed } from "vue";
import { getMsgPayload } from "../../utils/dispatcher";
import type { AgentEvent, AgentMessagePayload } from "../../types";
import { generateId } from "../../utils/crypto";

export function useLiveConsole(workspaceId: () => string, isExecuting: () => boolean | undefined, historyEvents: () => AgentEvent[] | undefined) {
  const liveEvents = ref<AgentEvent[]>([]);
  const isConnected = ref(false);
  let eventSource: EventSource | null = null;

  const displayEvents = computed(() => {
    if (liveEvents.value.length > 0) return liveEvents.value;
    if (isExecuting()) return [];
    return historyEvents() || [];
  });

  const handleAgentEvent = (ev: AgentEvent) => {
    // Assign a unique ID if it doesn't have one for better Vue list diffing
    if (!(ev as any).id) {
       (ev as any).id = generateId();
    }

    // Auto-clear old events if we detect a fresh automation start signal
    if (
      ev.type === "message" &&
      getMsgPayload(ev).content?.includes("▶ Booting automation:")
    ) {
      liveEvents.value = [];
      return;
    }

    // Handle streaming updates
    if (ev.type === "tool_stream") {
      const content = ev.payload as string;
      const lastEvent = liveEvents.value[liveEvents.value.length - 1];

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
  };

  const clearEvents = () => {
    liveEvents.value = [];
  };

  return {
    liveEvents,
    displayEvents,
    isConnected,
    connect,
    disconnect,
    clearEvents
  };
}
