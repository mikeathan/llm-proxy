import { getEventIcon } from "../constants/icons";
import type {
  AgentEvent,
  AgentStepStartPayload,
  AgentMessagePayload,
  AgentToolCallPayload,
  AgentToolResultPayload,
  AgentGuardrailViolationPayload,
  GuardrailBlockedPayload
} from "../types/dispatcher";
import { getRoleLabel } from "../domain/assistant";

/**
 * Utility helpers for working with AgentEvents and their payloads.
 */

export const isStepStart = (ev: AgentEvent): ev is AgentEvent & { payload: AgentStepStartPayload } => {
  return ev.type === 'step_start';
};

export const isMessage = (ev: AgentEvent): ev is AgentEvent & { payload: AgentMessagePayload } => {
  return ev.type === 'message';
};

export const isToolCall = (ev: AgentEvent): ev is AgentEvent & { payload: AgentToolCallPayload } => {
  return ev.type === 'tool_call';
};

export const isToolResult = (ev: AgentEvent): ev is AgentEvent & { payload: AgentToolResultPayload } => {
  return ev.type === 'tool_result';
};


/**
 * Helper to get typed payload in templates without 'as any'.
 */
export const getStepPayload = (ev: AgentEvent) => ev.payload as AgentStepStartPayload;
export const getMsgPayload = (ev: AgentEvent) => ev.payload as AgentMessagePayload;
export const getToolCallPayload = (ev: AgentEvent) => ev.payload as AgentToolCallPayload;
export const getToolResPayload = (ev: AgentEvent) => ev.payload as AgentToolResultPayload;
export const getViolationPayload = (ev: AgentEvent) => ev.payload as AgentGuardrailViolationPayload;
export const isGuardrailBlocked = (ev: AgentEvent) => ev.type === 'guardrail_blocked';
export const getBlockedPayload = (ev: AgentEvent) => ev.payload as GuardrailBlockedPayload;

/**
 * Formats a sequence of AgentEvents into a single plain-text log suitable for copy/paste.
 */
export const formatEventsToText = (events: AgentEvent[]): string => {
  return events
    .map((ev) => {
      if (ev.type === "step_start") {
        const ts = ev.timestamp ? ` [${new Date(ev.timestamp).toLocaleTimeString([], { hour12: false })}]` : "";
        return `Step ${getStepPayload(ev).step}${ts}`;
      }
      if (ev.type === "message") {
        const payload = getMsgPayload(ev);
        if (payload.content) {
          return `${getRoleLabel(payload.role).trim()}: ${payload.content}`;
        }
        return "";
      }
      if (ev.type === "tool_call") {
        const tc = getToolCallPayload(ev);
        return `${getEventIcon("tool_call")} Executing ${tc.function.name}...\n${tc.function.arguments}`;
      }
      if (ev.type === "tool_result") {
        const tr = getToolResPayload(ev);
        const hasError = !!tr.error || (typeof tr.result === "object" && tr.result !== null && "error" in tr.result);
        const resStr = hasError
          ? (tr.error || (tr.result as any).error)
          : typeof tr.result === "string"
            ? tr.result
            : JSON.stringify(tr.result, null, 2);
        return `${getEventIcon("tool_result")} ${tr.name} ${hasError ? "failed" : "finished"}\n${resStr}`;
      }
      if (ev.type === "guardrail_violation") {
        const vp = getViolationPayload(ev);
        return `${getEventIcon("guardrail_violation")} Guardrail Blocked: ${vp.tool}\n${vp.error}`;
      }
      if (ev.type === "guardrail_blocked") {
        const bp = ev.payload as GuardrailBlockedPayload;
        return `${getEventIcon("guardrail_blocked")} Guardrail Blocked — Awaiting Approval\nTool: ${bp.tool}\nReason: ${bp.reason}\nDecision ID: ${bp.decision_id}\nUse the approve/deny controls below to proceed.`;
      }
      return "";
    })
    .filter(Boolean)
    .join("\n\n");
};

import type { AssistantMessage } from "../types/assistant";

/**
 * Converts a tool_call AgentEvent into an AssistantMessage for the chat history.
 */
export const toolCallEventToMessage = (ev: AgentEvent): AssistantMessage => {
  const tc = getToolCallPayload(ev)
  return {
    role: 'assistant',
    content: '',
    tool_calls: [{
      id: '',
      type: 'function',
      function: {
        name: tc.function.name,
        arguments: tc.function.arguments,
      },
    }],
  }
}

/**
 * Converts a tool_result AgentEvent into an AssistantMessage for the chat history.
 * The full result object is preserved in toolResult for structured rendering.
 */
export const toolResultEventToMessage = (ev: AgentEvent): AssistantMessage => {
  const tr = getToolResPayload(ev)
  return {
    role: 'tool',
    content: '',
    tool_call_id: '',
    toolResult: tr,
  }
}
