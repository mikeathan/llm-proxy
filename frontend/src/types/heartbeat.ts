import type { NotifyConfig } from './automation'

// Mirrors the backend models.HeartbeatResult / HeartbeatConfig / HeartbeatState.
export type HeartbeatResult = 'quiet' | 'alert' | 'skipped_no_checks' | 'skipped_busy' | 'error'

export interface HeartbeatConfig {
  enabled: boolean
  every?: string
  model?: string
  notify?: NotifyConfig
}

export interface HeartbeatStatus {
  at: string
  result: HeartbeatResult
}

// What the Heartbeat panel edits; the connector is flattened so a select can bind to it.
export interface HeartbeatDraft {
  enabled: boolean
  every: string
  model: string
  connector: string
}

export interface HeartbeatState {
  config: HeartbeatConfig
  status?: HeartbeatStatus
  lane: 'local' | 'cloud'
  // A local model is woken by every check.
  wakes_local_model: boolean
  has_checks: boolean
}
