// A feed that has no separate headline (a Telegram post) uses the start of the text as the title, so the text under it would say the same thing
// again. This returns what the text adds after the title, or "" when it adds nothing. A title clipped with an ellipsis still counts as the start of it.
export function textBeyondTitle(title: string, text: string | undefined): string {
  const body = (text ?? '').trim()
  if (!body) return ''
  const head = title.trim().replace(/…$/, '').trimEnd()
  if (head && body.startsWith(head)) return body.slice(head.length).trim()
  return body
}
