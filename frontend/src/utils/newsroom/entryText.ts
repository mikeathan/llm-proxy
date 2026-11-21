// A feed that has no separate headline (a Telegram post) uses the start of the text as the title, so the text under it would say the same thing
// again. This returns what the text adds after the title, or "" when it adds nothing. The title has its whitespace collapsed to single spaces and the
// text keeps its line breaks, so both are compared with whitespace collapsed; a title clipped with an ellipsis still counts as the start of the text.
const collapse = (s: string): string => s.replace(/\s+/g, ' ').trim()

export function textBeyondTitle(title: string, text: string | undefined): string {
  const body = collapse(text ?? '')
  if (!body) return ''
  const head = collapse(title).replace(/…$/, '').trimEnd()
  if (head && body.startsWith(head)) return body.slice(head.length).trim()
  return body
}
