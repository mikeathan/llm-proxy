// LogLevel is the set of server log levels, derived from the canonical
// LOG_LEVELS const in constants/api.ts (single source of truth).
import { LOG_LEVELS } from '../constants/api'

export type LogLevel = (typeof LOG_LEVELS)[number]
