import type { NewsroomEntry } from '../../types/newsroom'

/** What a row calls a finding: the model's short headline when it wrote one, else the original title. */
export const entryHeadline = (entry: Pick<NewsroomEntry, 'title' | 'headline'>): string => entry.headline || entry.title
