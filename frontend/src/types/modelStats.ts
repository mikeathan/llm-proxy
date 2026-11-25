/** One day of model calls, local and cloud apart (GET /admin/api/model-stats). */
export interface ModelStatsDay {
  /** The day as YYYY-MM-DD in the server's time zone. */
  date: string
  local: number
  cloud: number
}

/** One model's use today. Tokens are what the provider reported; the speed is a local server's own figure and is absent for a cloud model. */
export interface ModelUse {
  model: string
  local: boolean
  calls: number
  failed: number
  prompt_tokens: number
  completion_tokens: number
  tokens_per_sec?: number
}

/** The calls that ended in one hour of today, local and cloud apart. */
export interface ModelStatsHour {
  local: number
  cloud: number
}

/** The last days oldest first, today's models busiest first, and today's calls by hour (24 entries from midnight; absent from a server that does not count them). */
export interface ModelStatsReport {
  days: ModelStatsDay[]
  today: ModelUse[]
  hours?: ModelStatsHour[]
}

/** One column of the week's chart: how tall the stack is, and how much of it is local and how much cloud (percent of the stack). */
export interface ChartColumn {
  date: string
  total: number
  heightPercent: number
  localPercent: number
  cloudPercent: number
}
