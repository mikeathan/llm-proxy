// Settings · Storage & logging · What is stored: the words of the data management card.
export const STORAGE_TITLE = 'What is stored'
export const STORAGE_INTRO = 'Everything this service keeps in its data folder, with its size. Something it does not recognise is listed under its own name. Sizes are measured at most once a minute.'
export const STORAGE_LOADING = 'Measuring the data folder'
export const STORAGE_FAILED = 'The data folder could not be measured.'
export const STORAGE_NOT_DESCRIBED = 'Not described: a file this version does not know'
export const STORAGE_STACK = 'Kept by the stack'
export const STORAGE_LEVEL_WORDS: Record<string, string> = {
  ok: 'Enough free space',
  low: 'The disk is getting full',
  critical: 'The disk is almost full: free some space',
  unknown: 'The free space could not be read',
}
export const storageTotalLine = (total: string, free: string, size: string): string => `${total} in the data folder · ${free} free of ${size}`
export const storageCapLine = (cap: string): string => `at most ${cap}`
export const storageRecordsLine = (n: number): string => `${n} ${n === 1 ? 'record' : 'records'}`
