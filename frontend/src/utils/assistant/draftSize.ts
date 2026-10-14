import { formatTokenCount } from '../format/units'

const LINE_BREAK = '\n'

/** "9 lines · 421 characters" — how big a draft is, for the size note and the full-screen editor. */
export function formatDraftSize(text: string): string {
  const lines = text.split(LINE_BREAK).length
  const chars = text.length
  return `${lines} line${lines === 1 ? '' : 's'} · ${formatTokenCount(chars)} character${chars === 1 ? '' : 's'}`
}
