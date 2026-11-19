import type { ModuleHealth, ModuleTraffic } from '../types/stacks'

// The words of the stacks screens. Layer and module titles come from the server; these are the fixed phrases around them.

export const STACK_LOAD_FAILED = 'The modules could not be read. Check that the server is running, then try again.'
export const STACK_SWITCH_FAILED = 'The switch could not be changed. Try again.'

export const STACKS_GROUP_LABEL = 'Stacks'
export const STACKS_INTRO = 'Every stack and whether anything of it is on. A stack with nothing on has no menu entry and no board on the dashboard; open its modules to switch one on.'
export const STACKS_MODULES_LINK = 'Open its modules'
export const stackStateLine = (on: number, total: number): string => (on === 0 ? `Off: none of its ${total} modules is on` : `On: ${on} of ${total} modules`)

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
export const MODULE_LAST_RAN_PREFIX = 'last ran'
/** The id a module tile carries, so a pointer to it can scroll there. */
export const moduleElementId = (id: string): string => `module-${id}`
export const goToModuleLabel = (title: string): string => `Go to ${title}`
export const MODULE_BLOCKED_WORD = 'Blocked'

export const stackSummaryLine = (on: number, total: number): string => `${on} of ${total} on`

export const STACK_OFF_FAILED = 'The stack could not be switched off. Some of its modules may still be on; check below, then try again.'
export const STACK_OFF_LABEL = 'Switch this stack off'
export const stackOffTitle = (title: string): string => `Switch ${title} off?`
export const STACK_OFF_MESSAGE =
  'Every module of this stack that is on is switched off, so it stops watching and sending. Your settings and what it already found are kept, and its menu entry and dashboard board are hidden until a module is switched on again.'
export const STACK_OFF_CONFIRM = 'Switch off'
export const STACK_IS_OFF_LINE = 'Off: no module of this stack is on. Switch one on below to bring the stack back.'

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

// What a module says about itself, and the coverage line on the Network overview.
export const MODULE_ABOUT_TITLE = 'What it does'
export const MODULE_ABOUT_LABELS = { watches: 'Watches', never: 'Never', shows_up: 'Shows up' } as const
export const MODULE_SETTINGS_LINK = 'Settings'
export const FOUND_BY_PREFIX = 'Found by'
export const FOUND_BY_OFF = 'off. Switch it on to keep this check running.'
export const COVERAGE_OPEN_MODULES = 'Open the Modules tab'
export const coverageLine = (on: number, total: number): string => `${on} of ${total} protections are watching`
export const COVERAGE_UNREAD = 'What is watching could not be read, so nothing is said about it here.'
export const COVERAGE_ALL_ON = 'Every protection is on.'
export const COVERAGE_OFF_INTRO = 'Off, and what each would watch:'
export const HELP_MODULES_TITLE = 'What each switch does'

/** The Radio page. Sensors is the page without a tab. */
export const RADIO_TITLE = 'Radio'
export const RADIO_SUBTITLE = 'Listen to the radio around this computer, and see what is there.'
export const RADIO_LISTENING_LABEL = 'Listening'
export const RADIO_TAB_LABELS = { sensors: 'Sensors', modules: 'Modules' } as const
