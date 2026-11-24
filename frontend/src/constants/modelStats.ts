export const MODEL_USE_LABEL = 'Model use'
export const MODEL_USE_DAYS = 7
export const MODEL_USE_POLL_MS = 60000
export const MODEL_USE_OPEN_LABEL = 'Show the model use'
export const MODEL_USE_FOLD_LABEL = 'Fold the model use'
export const MODEL_USE_LOCAL = 'Local'
export const MODEL_USE_CLOUD = 'Cloud'
export const MODEL_USE_DAYS_HEADING = (days: number): string => `Last ${days} days`
export const MODEL_USE_TODAY_HEADING = 'Today by model'
export const MODEL_USE_NONE_TODAY = 'No model calls today yet.'
export const MODEL_USE_LOADING = 'Reading the model use'
export const MODEL_USE_NO_CALLS = 'No calls today'
export const MODEL_USE_UNAVAILABLE = 'The model use could not be read.'
export const MODEL_USE_COLUMNS = { model: 'Model', calls: 'Calls', tokens: 'Tokens', speed: 'Speed' } as const
export const MODEL_USE_SPEED_UNIT = 'tok/s'
export const MODEL_USE_NO_VALUE = '—'
export const MODEL_USE_NO_TOKENS = 'tokens not reported'
export const MODEL_USE_FAILED = (n: number): string => `${n} failed`
export const MODEL_USE_CALLS_TODAY = (n: number): string => (n === 1 ? '1 call today' : `${n} calls today`)
export const MODEL_USE_CHART_SUMMARY = (days: number, local: number, cloud: number): string => `Model calls over the last ${days} days: ${local} local and ${cloud} cloud`
export const MODEL_USE_KIND = (local: boolean): string => (local ? MODEL_USE_LOCAL : MODEL_USE_CLOUD)
