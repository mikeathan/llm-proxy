import type { LoopStrategy } from './model'

// TriggerType is the automation trigger kind (cron / interval / manual).
export type TriggerType = 'cron' | 'interval' | 'manual'

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
}
