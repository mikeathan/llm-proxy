// Settings · Safety · Agent activity: the agent security record and the lock down switch.
export const LOCKDOWN_TITLE = 'Lock down'
export const LOCKDOWN_OFF =
  'For when something looks wrong: automations stop, agents lose the terminal, the network and MCP tools, connector messages are dropped and other browsers are signed out. Lifting it puts everything back.'
export const LOCKDOWN_REVOKE_LABEL = 'Also revoke every script token'
export const LOCKDOWN_REVOKE_HINT = 'Scripts that use a token stop working until you make a new one.'
export const LOCKDOWN_ENGAGE = 'Lock down now'
export const LOCKDOWN_RELEASE = 'Lift lock down'
export const LOCKDOWN_CONFIRM_TITLE = 'Lock down now?'
export const LOCKDOWN_CONFIRM_MESSAGE =
  'Automations will not start, agents cannot use the terminal, the network or MCP tools, and connector messages are dropped. Every other browser is signed out. You stay signed in here.'
export const LOCKDOWN_RELEASE_TITLE = 'Lift lock down?'
export const LOCKDOWN_RELEASE_MESSAGE = 'Automations, the terminal, the network, MCP tools and connector messages work again as before.'
export const LOCKDOWN_ON_TAG = 'On'

export const AGENTS_CARD_LOCKDOWN_OFF = 'Lock down is off'
export const AGENT_ACTIVITY_EMPTY = 'Nothing recorded yet'
export const DESTINATIONS_TITLE = 'Where automations fetch from'
