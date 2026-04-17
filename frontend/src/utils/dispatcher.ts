import type { 
  AgentEvent, 
  AgentStepStartPayload, 
  AgentMessagePayload, 
  AgentToolCallPayload, 
  AgentToolResultPayload,
  AgentGuardrailViolationPayload
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

/**
 * Formats a sequence of AgentEvents into a single plain-text log suitable for copy/paste.
 */
export const formatEventsToText = (events: AgentEvent[]): string => {
  return events
    .map((ev) => {
      if (ev.type === "step_start") {
        return `Step ${getStepPayload(ev).step}`;
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
        return `🛠️ Executing ${tc.function.name}...\n${tc.function.arguments}`;
      }
      if (ev.type === "tool_result") {
        const tr = getToolResPayload(ev);
        const resStr =
          typeof tr.result === "string"
            ? tr.result
            : JSON.stringify(tr.result, null, 2);
        return `✅ ${tr.name} finished\n${resStr}`;
      }
      if (ev.type === "guardrail_violation") {
        const vp = getViolationPayload(ev);
        return `🛑 Guardrail Blocked: ${vp.tool}\n${vp.error}`;
      }
      return "";
    })
    .filter(Boolean)
    .join("\n\n");
};
