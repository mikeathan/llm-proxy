import type { SessionBrief } from '../../types/assistant'

// clearRunningFlags returns a copy of the sessions with every `running` flag
// reset. It is used to heal sticky running state: when the authoritative
// backend reports nothing is executing, any locally-flagged running session is
// cleared so indicators can't get stuck "on".
export function clearRunningFlags(sessions: SessionBrief[]): SessionBrief[] {
  return sessions.map((s) => (s.running ? { ...s, running: false } : s))
}
