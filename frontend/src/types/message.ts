import type { AssistantMessage, Segment } from './assistant'

// Turn is a grouped unit of the conversation shown in the assistant chat /
// automation views. Matches agent-internal control messages injected by the
// backend (format errors, retry prompts, incomplete-response warnings,
// automation nag/retry signals); those use role:"user" so the model sees them as
// corrective feedback but must not create turn boundaries — or appear at all —
// in the UI. New entries here must stay in sync with prompts/templates.go.
export interface Turn {
  userMessage: string
  finalAnswer: string
  segments: Segment[]
  messages: AssistantMessage[]
  canceled?: boolean
}

// MessageBuilderOptions configures useMessageBuilder (chat vs automation,
// finalize mode). See utils/message/messageBuilder.ts.
export interface MessageBuilderOptions {
  source?: 'chat' | 'automation'
  // automation seeds a synthetic leading user message so groupTurns forms a
  // single clean turn (automation runs have no chat prompt of their own).
  headerMessage?: AssistantMessage
  // 'explicit'  → caller invokes finalize(reply) (chat: HTTP response).
  // 'lifecycle' → builder finalizes on lifecycle{phase:'completed'}
  //               (automation: Hermes-aligned, loop-announced completion).
  finalizeOn?: 'explicit' | 'lifecycle'
}
