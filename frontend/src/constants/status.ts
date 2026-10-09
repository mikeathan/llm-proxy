import type { StatusState } from '../types/ui'

// The one state → token map (plan Phase 5, StatusTag): a borderless tinted band
// with a square dot. Text always carries the meaning; colour only reinforces it.
export const STATUS_TAG_CLASS: Record<StatusState, string> = {
  success: 'text-state-success bg-state-success/10',
  // Completed, but something needs a look (e.g. a report not delivered). No warning token exists: it shares the
  // amber state-running pair, already contrast-checked in every preset, and the label always says "warnings".
  warning: 'text-state-running bg-state-running/10',
  running: 'text-state-running bg-state-running/10',
  queued: 'text-state-queued bg-transparent',
  error: 'text-state-error bg-state-error/10',
  info: 'text-accent-info-text bg-accent-info/[0.12]',
  neutral: 'text-muted bg-surface-raised',
}

// Queued is the one state drawn with a hollow dot: waiting, not yet active.
export const HOLLOW_DOT_STATES: readonly StatusState[] = ['queued']
