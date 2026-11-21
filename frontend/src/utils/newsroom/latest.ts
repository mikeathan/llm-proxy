import type { Headline, NewsroomDay, NewsroomWatch } from '../../types/newsroom'

/** The newest findings across every watch and day, newest first: each watch's current finding and the earlier ones of its day. */
export function latestHeadlines(days: readonly NewsroomDay[], limit: number): Headline[] {
  const all = days.flatMap((d) => d.watches.flatMap((t) => [t.current, ...(t.earlier ?? [])].map((entry) => ({ watch: t.watch, entry }))))
  return all.sort((a, b) => Date.parse(b.entry.at) - Date.parse(a.entry.at)).slice(0, limit)
}

/** The watches whose last check could not read a source. */
export const failingWatches = (watches: readonly NewsroomWatch[]): NewsroomWatch[] => watches.filter((t) => t.enabled && t.last_status === 'failing')

