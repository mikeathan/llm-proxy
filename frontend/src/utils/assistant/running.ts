import type { SessionBrief } from '../../types/assistant'

// clearRunningFlags returns a copy of the sessions with every `running` flag
// reset. It is used to heal sticky running state: when the authoritative
// backend reports nothing is executing, any locally-flagged running session is
// cleared so indicators can't get stuck "on".
export function clearRunningFlags(sessions: SessionBrief[]): SessionBrief[] {
  return sessions.map((s) => (s.running ? { ...s, running: false } : s))
}

// markRunningConversation returns a copy of the sessions with exactly the
// session whose ID matches `conversationId` flagged running and every other
// session unflagged. It is the per-session counterpart to clearRunningFlags:
// the backend reports which conversation is executing (via /active-runs), and
// the UI restores the per-session running indicator from it after a refresh.
// Returns the same array identity when nothing changes (idempotent).
export function markRunningConversation(sessions: SessionBrief[], conversationId: string): SessionBrief[] {
  if (!conversationId) return sessions
  let changed = false
  const next = sessions.map((s) => {
    const running = s.id === conversationId
    if (s.running === running) return s
    changed = true
    return { ...s, running }
  })
  return changed ? next : sessions
}

// reconcileRunningSessions applies the backend's authoritative running
// conversation to the session list:
//   - non-empty conversationId: exactly that session is flagged running, every
//     other session is unflagged (see markRunningConversation);
//   - empty conversationId (nothing running): every running flag is cleared.
// Returns the same array identity when nothing changes (idempotent). This is the
// single pure source for restoring the per-session running indicator from
// /active-runs, so the caller keeps only a `lastRunningConversationId` string
// (the empty value clears it) rather than a sticky flag.
export function reconcileRunningSessions(sessions: SessionBrief[], conversationId: string): SessionBrief[] {
  if (conversationId) return markRunningConversation(sessions, conversationId)
  return clearRunningFlags(sessions)
}
