import type { NewsroomEntry } from '../../types/newsroom'

const FALLBACK_MAX = 90
const FIRST_SENTENCE = /^(.{15,}?[.!?])(?:\s|$)/
// Emoji and pictographs, variation selectors and joiners; stripped only from the fallback headline, never from stored text.
const PICTOGRAPHS = /[\p{Extended_Pictographic}\u{200d}\u{fe0f}]/gu

/**
 * A readable stand-in for a headline when the model wrote none: the post's first sentence, without links or emoji, cut at a word at 90
 * characters. A post that gives nothing readable after that keeps its own title.
 */
export function fallbackHeadline(title: string): string {
  const plain = title.replace(/https?:\/\/\S+/g, '').replace(PICTOGRAPHS, '').replace(/\s+/g, ' ').trim()
  const sentence = FIRST_SENTENCE.exec(plain)?.[1] ?? plain
  if (sentence.length <= FALLBACK_MAX) return sentence || title
  const cut = sentence.slice(0, FALLBACK_MAX)
  return cut.slice(0, Math.max(cut.lastIndexOf(' '), 1)).trimEnd() + '…'
}

/** What a row calls a finding: the model's short headline when it wrote one, else a short readable line from the original title. */
export const entryHeadline = (entry: Pick<NewsroomEntry, 'title' | 'headline'>): string => entry.headline || fallbackHeadline(entry.title)
