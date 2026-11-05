import type { ModuleHealth, ModuleTraffic } from '../types/stacks'

// The words of the stacks screens. Layer and module titles come from the server; these are the fixed phrases around them.

export const STACK_LOAD_FAILED = 'The modules could not be read. Check that the server is running, then try again.'
export const STACK_SWITCH_FAILED = 'The switch could not be changed. Try again.'

export const STACKS_GROUP_LABEL = 'Stacks'

export const STACK_MODULES_INTRO = 'Every module of this stack, in layers. A module builds on the ones below it. Switch them here; details stay on each module’s own page.'

/** What a module sends, in the status line. */
export const TRAFFIC_WORDS: Record<ModuleTraffic, string> = {
  nothing: 'sends nothing',
  lan: 'LAN',
  internet: 'internet',
  message: 'sends a message',
  listens: 'listens',
}

export const HEALTH_WORDS: Record<ModuleHealth, string> = {
  working: 'Working',
  stopped: 'Stopped',
}

export const MODULE_OFF_WORD = 'Off'
export const MODULE_BLOCKED_WORD = 'Blocked'

export const stackSummaryLine = (on: number, total: number): string => `${on} of ${total} on`

export const LEFT_BLOCKED_PREFIX = 'Now waiting for it: '

/** The Agents page. Activity is the page without a tab. */
export const AGENTS_TITLE = 'Agents'
export const AGENTS_SUBTITLE = 'What the agents on this computer did, and what to do if something looks wrong.'
export const AGENTS_ACTIVITY_LABEL = 'Activity'
export const AGENTS_TAB_LABELS = { modules: 'Modules' } as const

/** Where a module's switch used to be, on its detail card. */
export const MODULE_SWITCHED_ON_NOTE = 'Switched on.'
export const MODULE_SWITCHED_OFF_NOTE = 'Switched off.'
export const MODULES_LINK_LABEL = 'Switch it on the Modules tab'
