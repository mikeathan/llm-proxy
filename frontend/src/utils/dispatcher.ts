import type { 
  AgentEvent, 
  AgentStepStartPayload, 
  AgentMessagePayload, 
  AgentToolCallPayload, 
  AgentToolResultPayload 
} from "../types/dispatcher";

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
