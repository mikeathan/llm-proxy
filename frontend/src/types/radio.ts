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
