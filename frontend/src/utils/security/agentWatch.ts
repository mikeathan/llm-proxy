import { formatAbsoluteTime } from '../format/time'
import type { AgentWatchAutomation, AgentWatchEvent } from '../../types/security'

const CHAT_SOURCE = 'a chat'

/** One entry of the agent security record in one plain line. The names in it are plain tokens the server vetted. */
export function agentEventText(e: AgentWatchEvent): string {
  switch (e.kind) {
    case 'tool_refused':
      return `The safety rules refused ${e.tool ?? 'a tool'}`
    case 'approval_asked':
      return `Asked you to approve ${e.tool ?? 'a tool'}`
    case 'new_destination':
      return `Fetched from a new host: ${e.host ?? 'unknown'}`
    case 'secret_found': {
      const what = e.detail ? e.detail.charAt(0).toUpperCase() + e.detail.slice(1) : 'A key or token'
      return `${what} was left in ${e.where ?? 'a file'}`
    }
    default:
      return `Recorded: ${e.kind}`
  }
}

/** Where an entry came from: the automation and its workspace, a chat, or (for a secret the nightly pass found) the workspace. */
export const agentEventSource = (e: AgentWatchEvent): string =>
  e.kind === 'secret_found' ? e.workspace : `${e.automation || CHAT_SOURCE} · ${e.workspace}`

/** What the destination watch does for one automation, in one line. */
export function automationLine(a: AgentWatchAutomation): string {
  if (a.learning) return `${a.automation}: learning its usual hosts until ${formatAbsoluteTime(a.learning_until)}`
  if (a.browses) return `${a.automation}: reaches many hosts, so a new one is only recorded`
  return `${a.automation}: ${a.hosts} usual ${a.hosts === 1 ? 'host' : 'hosts'}; a new one raises an alert`
}
