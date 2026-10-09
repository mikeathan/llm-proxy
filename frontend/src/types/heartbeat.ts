import type { NotifyConfig } from './automation'

// Mirrors the backend models.HeartbeatResult / HeartbeatConfig / HeartbeatState.
export type HeartbeatResult =
  | 'quiet'
  | 'alert'
  | 'alert_not_delivered'
  | 'skipped_no_checks'
  | 'skipped_busy'
  | 'skipped_outside_hours'
  | 'error'

export interface HeartbeatConfig {
  enabled: boolean
  every?: string
  model?: string
  notify?: NotifyConfig
  // "HH:MM-HH:MM" in server time; absent means checks run all day.
  active_hours?: string
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
  // The two ends of active_hours as <input type="time"> values; both empty means all day.
  activeFrom: string
  activeTo: string
}

export interface HeartbeatState {
  config: HeartbeatConfig
  status?: HeartbeatStatus
  lane: 'local' | 'cloud'
  // A local model is woken by every check.
  wakes_local_model: boolean
  has_checks: boolean
}
