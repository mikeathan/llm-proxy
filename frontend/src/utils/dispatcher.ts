import type {
  AgentEvent,
  AgentStepStartPayload,
  AgentMessagePayload,
  AgentToolCallPayload,
  AgentToolResultPayload,
  AgentGuardrailViolationPayload,
} from "../types/dispatcher";

/**
 * Typed payload accessors for AgentEvents. Templates and message formatters use
 * these instead of `as any` casts on raw events. (The old type guards and the
 * plain-text `formatEventsToText` log formatter had no consumers and were
 * removed.)
 */
export const getStepPayload = (ev: AgentEvent): AgentStepStartPayload => ev.payload as AgentStepStartPayload;
export const getMsgPayload = (ev: AgentEvent): AgentMessagePayload => ev.payload as AgentMessagePayload;
export const getToolCallPayload = (ev: AgentEvent): AgentToolCallPayload => ev.payload as AgentToolCallPayload;
export const getToolResPayload = (ev: AgentEvent): AgentToolResultPayload => ev.payload as AgentToolResultPayload;
export const getViolationPayload = (ev: AgentEvent): AgentGuardrailViolationPayload => ev.payload as AgentGuardrailViolationPayload;
