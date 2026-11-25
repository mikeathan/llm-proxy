import { ALERT_PROBLEM_WORDS, sourcesDownWords } from '../../constants/newsroom'
import type { NewsroomWatch } from '../../types/newsroom'

const needsLook = (s: NewsroomWatch['last_status']): s is keyof typeof ALERT_PROBLEM_WORDS => s !== undefined && s in ALERT_PROBLEM_WORDS

/** One short line per switched-on watch that needs a look ("Go: source failing"); none when all is well, so the Alerts tab stays quiet. */
export function alertProblems(watches: readonly NewsroomWatch[]): string[] {
  return watches.flatMap((w) => {
    if (!w.enabled) return []
    if (needsLook(w.last_status)) return [`${w.name}: ${ALERT_PROBLEM_WORDS[w.last_status]}`]
    const down = w.last_problems?.length ?? 0
    return down > 0 ? [`${w.name}: ${sourcesDownWords(down)}`] : []
  })
}
