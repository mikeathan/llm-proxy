import { STANDING_NONE_OPEN, STANDING_NO_HISTORY, STANDING_TOO_SHORT, STANDING_TREND_WORDS } from '../../constants/netaudit'
import type { NetworkStanding } from '../../types/netaudit'

const plural = (n: number, one: string, many: string): string => `${n} ${n === 1 ? one : many}`

/** How long the open problems have lasted, in one sentence; says so when none has a history. */
export function ageSentence(s: NetworkStanding): string {
  if (!s.tracked) return STANDING_NONE_OPEN
  const base = `${plural(s.tracked, 'open problem has', 'open problems have')} been there for ${plural(s.median_days, 'day', 'days')} in the middle, the oldest for ${plural(s.oldest_days, 'day', 'days')}.`
  const parts = [base]
  if (s.over_month) parts.push(`${s.over_month} ${s.over_month === 1 ? 'has' : 'have'} lasted over a month.`)
  if (s.came_back) parts.push(`${s.came_back} came back after going away.`)
  return parts.join(' ')
}

/** Last week against the one before, in words; no comparison, and a reason, while there is too little history. */
export function trendSentence(s: NetworkStanding): string {
  if (!s.history_since) return STANDING_NO_HISTORY
  const week = `In the last 7 days ${plural(s.week.opened, 'problem appeared', 'problems appeared')} and ${plural(s.week.resolved, 'went', 'went')} away.`
  if (s.trend === 'unknown') return `${week} ${STANDING_TOO_SHORT}`
  return `${week} ${STANDING_TREND_WORDS[s.trend]}`
}
