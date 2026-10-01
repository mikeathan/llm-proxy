import type { LaneKey, LaneKind } from '../types/assistant'

// Display names for the backend run-scheduler enums (runlane.Kind / LaneKey),
// shared by every surface that shows running work (run pill, Overview).
export const RUN_KIND_LABELS: Record<LaneKind, string> = {
  automation: 'Automation',
  interactive: 'Assistant',
  inbound: 'API caller',
}

export const LANE_LABELS: Record<LaneKey, string> = { local: 'Local', cloud: 'Cloud' }
