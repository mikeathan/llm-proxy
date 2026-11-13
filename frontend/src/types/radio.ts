/** GET /admin/api/radio/sensors: the radios on this computer and what each can do. Every word is the server's fixed one. */

export type RadioKind = 'wifi' | 'bluetooth' | 'ieee802154'

export type RadioMode = 'managed' | 'monitor' | 'other'

/** What the stack does with a capability: the four answers the Sensors page groups by. */
export type CapabilityUse = 'used' | 'unused_by_design' | 'unused_not_built' | 'found_no_driver'

export interface RadioCapability {
  id: string
  title: string
  use: CapabilityUse
  /** False until the privileged probe has read it. */
  known: boolean
  note: string
}

export interface RadioSensor {
  id: string
  kind: RadioKind
  bus?: string
  driver?: string
  usb_id?: string
  mode?: RadioMode
  eligible: boolean
  why: string
  capabilities: RadioCapability[]
}

export interface RadioView {
  enabled: boolean
  sensors: RadioSensor[]
  /** A fixed phrase, set when there is nothing to list. */
  reason?: string
  /** How many capabilities are found and not used yet. */
  unused: number
}

/** GET /admin/api/radio/listening: the run, as counts and words only (never an address or a name). */
export interface RadioWindow {
  channel: number
  frames: number
  recorded: number
  unread: number
  stop: string
  kernel_seen: number
  kernel_dropped: number
  elapsed_ms: number
  on_channel: number
  off_channel: number
  channel_matched: boolean
  at: string
}

export type RadioEnded = 'no_session' | 'time' | 'frames' | 'failing' | 'stopped' | 'switched_off' | 'stack_stopped'

export interface RadioPlan {
  channels: number[]
  window_seconds: number
  cycle_seconds: number
}

export interface RadioListenState {
  listening: boolean
  adapter?: string
  started?: string
  until?: string
  cycles: number
  frames: number
  windows: RadioWindow[]
  ended?: RadioEnded
  last_error?: string
  plan: RadioPlan
}

export interface RadioListenRequest {
  action: string
  params: unknown
}

/** GET /admin/api/radio/listening/plan: the one list a session approves and the run asks. */
export interface RadioPlanView {
  adapter: string
  plan: RadioPlan
  requests: RadioListenRequest[]
  max_hours: number
}

export interface RadioOwnerDevice {
  id: string
  label: string
  visits: number
  first_seen: string
  last_seen: string
}

/** A bounded session as the core lists it (Constitution I.17). */
export interface HelperSession {
  id: string
  stack: string
  label: string
  started: string
  expires: string
  uses: number
  max_uses: number
}
