import type { LoopStrategy } from './model'
import type { NetworkGrant } from './admin'

// MemoryMode opts an automation into hot-memory injection ('' = off). Mirrors
// the backend models.MemoryMode; 'hot+hints' is not shipped yet.
export type MemoryMode = '' | 'off' | 'hot'

// NotifyConfig delivers an automation's report through a communication
// connector (backend models.NotifyConfig). dedup_days 0 = the backend default.
export interface NotifyConfig {
  connector: string
  dedup?: boolean
  dedup_days?: number
  send_empty?: boolean
}

// TriggerType is the automation trigger kind (cron / interval / manual).
export type TriggerType = 'cron' | 'interval' | 'manual'

// AutomationPayload is what the create / update endpoints accept
// (POST …/workspaces/{ws}/automations, PUT …/automations/{name}).
export interface AutomationPayload {
  name: string
  trigger: { type: TriggerType; value: string }
  task_file: string
  strategy: string
  model: string
  loop_strategy: LoopStrategy
  network_grant: NetworkGrant
  memory_mode: MemoryMode
  // null clears delivery on update.
  notify: NotifyConfig | null
  skip_if_busy: boolean
}

// AutomationFormData is the editable shape of the automation form. See
// composables/automation/useAutomationForm.ts.
export interface AutomationFormData {
  name: string
  triggerType: TriggerType
  triggerValue: string
  taskFile: string
  strategy: string
  model: string
  loopStrategy: LoopStrategy
  networkGrant: NetworkGrant
  memoryMode: MemoryMode
  // Delivery: notifyConnector '' = results are not sent anywhere.
  notifyConnector: string
  notifyDedup: boolean
  // Kept as typed text so an empty field (= default retention) stays distinct from 0.
  notifyDedupDays: string
  notifySendEmpty: boolean
  skipIfBusy: boolean
}
