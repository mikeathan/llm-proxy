// Which bundled artwork a section gets. A topic's category is a free label ("Technology", "UK politics"), so the choice reads the words, category first,
// and falls back to a plain newspaper plate. Pure, so it is tested without a browser.
import type { PlateKind } from '../../types/newsroom'

const RULES: [PlateKind, RegExp][] = [
  ['economy', /econom|market|financ|business|stock|trade|money/i],
  ['tech', /tech|\bai\b|software|cyber|computer|gadget|internet/i],
  ['politics', /politic|government|election|law|parliament|congress/i],
  ['science', /scien|climate|health|space|environment|research|medic/i],
  ['world', /world|international|global|europe|asia|africa|america|middle east/i],
]

export function plateKind(category: string | undefined, name: string): PlateKind {
  for (const text of [category ?? '', name]) {
    const hit = RULES.find(([, re]) => re.test(text))
    if (hit) return hit[0]
  }
  return 'paper'
}
