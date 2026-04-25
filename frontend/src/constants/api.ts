// API endpoint constants for the admin panel
export const API_BASE = '/admin/api'

export const API_ENDPOINTS = {
  state: `${API_BASE}/state`,
  metrics: `${API_BASE}/metrics`,
  logs: `${API_BASE}/logs`,
  appLogs: `${API_BASE}/app-logs/tail`,
  logLevel: `${API_BASE}/log-level`,
  models: `${API_BASE}/models`,
  start: `${API_BASE}/start`,
  stop: `${API_BASE}/stop`,
  config: `${API_BASE}/config`,
  mcp: `${API_BASE}/mcp`,
  providerModels: `${API_BASE}/providers/models`,
  providerManifests: `${API_BASE}/providers/manifests`,
  testConnection: `${API_BASE}/providers/test`,
  secretsKeys: `${API_BASE}/secrets/keys`,
  restart: `${API_BASE}/system/restart`,
  hostSettings: `${API_BASE}/host`,
} as const

// Polling interval in milliseconds for state/metrics refresh
export const POLL_INTERVAL_MS = 5000

// Default log level
export const DEFAULT_LOG_LEVEL = 'INFO' as const

// Log level options
export const LOG_LEVELS = ['DEBUG', 'INFO', 'WARN', 'ERROR'] as const
export type LogLevel = (typeof LOG_LEVELS)[number]
