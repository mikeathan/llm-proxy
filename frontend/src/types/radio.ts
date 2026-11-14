/** GET /admin/api/radio/sensors: the radios on this computer and what each can do. Every word is the server's fixed one. */

export type RadioKind = 'wifi' | 'bluetooth' | 'ieee802154'

export type RadioMode = 'managed' | 'monitor' | 'other'

/** What the stack does with a capability. `unsupported` is a fact about the hardware (its own probe said so); `ready` can be used and is not running. */
export type CapabilityUse = 'used' | 'ready' | 'unsupported' | 'unused_by_design' | 'unused_not_built' | 'found_no_driver'

export interface RadioCapability {
  id: string
  title: string
  use: CapabilityUse
  /** False until the privileged probe has read it. */
  known: boolean
  note: string
}

/** What the adapter's own probe said, once it has been read (the first time the stack listens). */
export interface RadioProbe {
  read_at: string
  modes: string[]
  bands: string[]
  standards: string[]
  monitor: boolean
}

export type RadioRole = 'listening' | 'can_listen' | 'wont_use' | 'no_driver'

export interface RadioSensor {
  id: string
  kind: RadioKind
  bus?: string
  driver?: string
  usb_id?: string
  mode?: RadioMode
  eligible: boolean
  why: string
  role: RadioRole
  probe?: RadioProbe
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
  /** How many of the recorded frames one of the owner's devices sent. */
  own_frames: number
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

export type RadioClass = 'infrastructure' | 'personal' | 'neighbour' | 'transient' | 'unknown'

export interface RadioOwnerDevice {
  id: string
  label: string
  visits: number
  first_seen: string
  last_seen: string
  class: RadioClass
  /** Who decided the class: the owner's own word, or the classifier's rules. */
  class_by: 'owner' | 'rules'
}

/** GET /admin/api/radio/air: other people's devices as counts by class, never a device. */
export interface RadioAir {
  rules: { version: number; provisional: boolean; window_days: number; min_cycles_listened: number }
  listened_cycles: number
  counts: Record<RadioClass, number>
  total: number
  changes: { day: string; from: string; to: string; version: number; reason: string; n: number }[]
}

export interface RadioNetworkAP {
  id: string
  name: string
  channel: number
  security: string
  confirmed: boolean
  last_seen: string
}

export interface RadioNetworks {
  names: string[]
  access_points: RadioNetworkAP[]
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

/** How strong a device's signal is, against the rules' thresholds. */
export type RadioSignal = 'strong' | 'medium' | 'weak' | 'unknown'

/** One device of someone else, by a short id (four bytes of a keyed tag): never an address. */
export interface RadioAirDevice {
  id: string
  class: RadioClass
  /** The fixed reason word the rules gave. */
  why: string
  signal: RadioSignal
  first_seen: string
  last_seen: string
  cycles_heard: number
  need_heard: number
  retry_percent: number
  channels: number[]
  random_address: boolean
  joined_own: boolean
}

/** GET /admin/api/radio/air/devices */
export interface RadioAirDevices {
  listened_cycles: number
  min_listened_cycles: number
  total: number
  devices: RadioAirDevice[]
}

export type RadioAirSort = 'last_heard' | 'cycles' | 'signal' | 'retries'

/** GET /admin/api/radio/air/airwaves: counts only. */
export interface RadioAirwaves {
  channels: { channel: number; devices: number }[]
  classes: { class: RadioClass; devices: number; frames: number; retry_percent: number }[]
  routers: { name: string; channel: number }[]
  near_unknown: number
  listened_cycles: number
}

export type RadioHourState = 'here' | 'away' | 'not_listening'

export interface RadioPresenceDevice {
  id: string
  label: string
  state: RadioHourState
  signal?: RadioSignal
  last_heard?: string
  joined_own: boolean
  hours: { hour: string; state: RadioHourState; signal?: RadioSignal }[]
}

/** GET /admin/api/radio/devices/mine/presence */
export interface RadioPresence {
  devices: RadioPresenceDevice[]
}

export type RadioTier = 'seen' | 'inferred' | 'estimated'

/** GET /admin/api/radio/digest: what the stack found, as fixed words and counts. No text a device chose is ever in it. */
export interface RadioDigest {
  listening: { cycles: number; min_cycles: number; window_days: number; cycles_last_24h: number }
  own: { devices: number; access_points: number; confirmed_access_points: number }
  others: { class: RadioClass; count: number; tier: RadioTier }[]
  findings: { id: string; tier: RadioTier; count: number; claim: string }[]
  could_not_see: { id: string; reason: string }[]
}
