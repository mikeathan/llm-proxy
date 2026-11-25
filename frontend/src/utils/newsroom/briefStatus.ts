import type { BriefEdition, BriefIssue, BriefStatus } from '../../types/newsroom'

const sameDay = (a: Date, b: Date): boolean => a.toDateString() === b.toDateString()
const clock = (d: Date): string => d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })

/** The next moment the edition is made automatically after `now`: its ready time on the next day it runs; null when it runs on no day (paused). */
export function nextReady(edition: Pick<BriefEdition, 'ready_by' | 'days'>, now: Date): Date | null {
  const [h, m] = edition.ready_by.split(':').map(Number)
  if (h === undefined || m === undefined || Number.isNaN(h) || Number.isNaN(m)) return null
  const days = edition.days.length ? edition.days : [0, 1, 2, 3, 4, 5, 6]
  for (let i = 0; i < 8; i++) {
    const d = new Date(now.getFullYear(), now.getMonth(), now.getDate() + i, h, m)
    if (d.getTime() > now.getTime() && days.includes(d.getDay())) return d
  }
  return null
}

const whenWords = (d: Date, now: Date): string => {
  const tomorrow = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1)
  if (sameDay(d, now)) return `today at ${clock(d)}`
  if (sameDay(d, tomorrow)) return `tomorrow at ${clock(d)}`
  return `${d.toLocaleDateString(undefined, { weekday: 'long' })} at ${clock(d)}`
}

export function briefStatus(edition: Pick<BriefEdition, 'ready_by' | 'days' | 'enabled'>, issue: Pick<BriefIssue, 'made_at' | 'ready_for'> | null, now = new Date()): BriefStatus {
  const next = edition.enabled === false ? null : nextReady(edition, now)
  const nextWords = next ? `The next one is written automatically ${whenWords(next, now)}.` : 'This brief is paused, so none is written automatically.'
  if (!issue) return { tone: 'none', title: 'No brief written yet', detail: nextWords, next: nextWords }
  const made = new Date(issue.made_at)
  const how = issue.ready_for ? 'written automatically' : 'written by hand'
  if (sameDay(made, now)) return { tone: 'fresh', title: `Today's brief`, detail: `It was ${how} at ${clock(made)}. Nothing to press. ${nextWords}`, next: nextWords }
  const day = made.toLocaleDateString(undefined, { weekday: 'long', day: 'numeric', month: 'long' })
  return { tone: 'old', title: `This brief is from ${day}`, detail: `It is not today's. ${nextWords} You do not have to press anything.`, next: nextWords }
}
