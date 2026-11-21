import type { NewsroomDay, NewsroomEntry } from '../../types/newsroom'

/** Every entry of a day as one list, newest first, whichever watch found it. The server groups by watch; the page reads a day as one timeline. */
export function dayEntries(day: NewsroomDay): NewsroomEntry[] {
  return day.watches
    .flatMap((w) => [w.current, ...(w.earlier ?? [])])
    .sort((a, b) => (a.at === b.at ? b.id - a.id : a.at < b.at ? 1 : -1))
}

/** The ids of the unread entries in a list. */
export const unreadIds = (entries: readonly NewsroomEntry[]): number[] => entries.filter((e) => !e.read).map((e) => e.id)
