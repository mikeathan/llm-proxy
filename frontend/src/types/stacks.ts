import type { RouteLocationNamedRaw } from 'vue-router'

// The stacks of switchable modules (ADR-0028): a stack holds layers, a layer holds modules. These mirror /admin/api/stacks.

export const STACK_IDS = ['network', 'agents'] as const
export type StackId = (typeof STACK_IDS)[number]

/** What a module sends or opens, in the server's words. */
export type ModuleTraffic = 'nothing' | 'lan' | 'internet' | 'message' | 'listens'

/** How a module that is switched on is doing; absent while it is off or has no monitor. */
export type ModuleHealth = 'working' | 'stopped'

/** What a module says about itself, in the server's words: what it watches, what it never does, where its results appear. */
export interface ModuleAbout {
  watches: string
  never: string
  shows_up: string
  /** True when the stack's Settings tab has a card for the module's details. */
  settings?: boolean
}

export interface StackModule {
  id: string
  title: string
  purpose: string
  about: ModuleAbout
  /** The check ids of the findings only this module produces. */
  finds: string[]
  sends: ModuleTraffic
  step_up: boolean
  enabled: boolean
  health?: ModuleHealth
  /** Plain words for what stops the module working now ("needs Presence check"); absent when nothing does. */
  blocked?: string
  /** The ids of the modules this one needs that are off, so the page can point at them; absent when nothing it needs is off. */
  blocked_by?: string[]
  needs: string[]
  /** When the module's background work last ran (ISO time); absent before its first run or while it is off. */
  last_run?: string
  /** The stack's own fixed phrase for what is wrong with a module that is on; absent when nothing is. */
  problem?: string
}

export interface StackLayer {
  id: string
  title: string
  purpose?: string
  modules: StackModule[]
}

export interface StackAction {
  id: string
  title: string
  purpose: string
}

export interface StackCounts {
  on: number
  total: number
  stopped: number
  blocked: number
}

/** One stack with its layers, top first. */
export interface StackView {
  id: StackId
  title: string
  purpose: string
  summary: StackCounts
  layers: StackLayer[]
  actions: StackAction[]
}

/** The answer to a switch: the stack as it is now, and the titles of the modules the change left without something they need. */
export interface SwitchedStack extends StackView {
  blocked: string[]
}

/** The colour family of a module's status line; the words always carry the meaning too. */
export type ModuleTone = 'success' | 'warning' | 'muted'

export interface ModuleStatus {
  text: string
  tone: ModuleTone
}

/** One tab of a stack's page; `to` is a typed route builder's result. */
export interface StackTabItem {
  id: string
  label: string
  to: RouteLocationNamedRaw
}
