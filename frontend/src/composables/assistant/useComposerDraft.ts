// The text in a workspace's message box, kept across the chat being taken down. A 401 sends the user to the sign-in page,
// which has no shell, so the chat unmounts with a half-written message in its box; the router brings them back after
// signing in. In memory only (the page does not reload): a draft can hold anything, so it never reaches storage, and it
// is gone when the tab is.
const drafts = new Map<string, string>()

/** Remembers what is in the box; nothing (or only blanks) forgets the draft. */
export function stashDraft(workspaceId: string, text: string): void {
  if (text.trim()) drafts.set(workspaceId, text)
  else drafts.delete(workspaceId)
}

/** The draft left in the box when the chat was last taken down, or "". */
export function peekDraft(workspaceId: string): string {
  return drafts.get(workspaceId) ?? ""
}
