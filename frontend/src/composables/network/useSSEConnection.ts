import { ref } from "vue";

const DEATH_SPIRAL_THRESHOLD = 5;
const DEATH_SPIRAL_DELAY_MS = 30_000;

interface SSEConnectionOptions {
  /** URL getter — called on each connect attempt (supports dynamic workspaceId). */
  url: () => string;
  /** Called on each parsed `agent_update` event. */
  onMessage?: (event: any) => void;
}

/**
 * Reusable SSE connection with dedup, built-in reconnect, and death-spiral
 * protection (closes after 5+ consecutive errors, reconnects after 30s).
 *
 * Consumers own the domain-specific event handling (guardrail state,
 * live events, etc.) — this composable owns the transport layer only.
 */
export function useSSEConnection(options: SSEConnectionOptions) {
  const isConnected = ref(false);
  let eventSource: EventSource | null = null;
  let reconnectAttempts = 0;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;

  function connect() {
    if (eventSource) eventSource.close();
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }

    const url = options.url();
    eventSource = new EventSource(url);

    eventSource.addEventListener("ping", () => {
      isConnected.value = true;
      reconnectAttempts = 0;
    });

    eventSource.addEventListener("agent_update", (e) => {
      try {
        options.onMessage?.(JSON.parse(e.data));
      } catch (err) {
        console.error("Failed to parse SSE event", err);
      }
    });

    eventSource.onerror = () => {
      isConnected.value = false;
      reconnectAttempts++;
      // Let the browser's built-in reconnect handle transient disconnects.
      // Only intervene on death spiral (5+ consecutive failures = 404/401).
      if (reconnectAttempts >= DEATH_SPIRAL_THRESHOLD) {
        eventSource?.close();
        eventSource = null;
        reconnectTimer = setTimeout(connect, DEATH_SPIRAL_DELAY_MS);
      }
    };
  }

  function disconnect() {
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
    reconnectAttempts = 0;
    if (eventSource) {
      eventSource.close();
      eventSource = null;
    }
    isConnected.value = false;
  }

  function reset() {
    disconnect();
    reconnectAttempts = 0;
  }

  return { isConnected, connect, disconnect, reset };
}
