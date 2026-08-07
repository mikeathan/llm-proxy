// API endpoint constants for the admin panel
export const API_BASE = '/admin/api'

export const API_ENDPOINTS = {
  state: `${API_BASE}/state`,
  metrics: `${API_BASE}/metrics`,
  logs: `${API_BASE}/logs`,
  appLogs: `${API_BASE}/app-logs/tail`,
  logLevel: `${API_BASE}/log-level`,
  models: `${API_BASE}/models`,
  modelsAll: `${API_BASE}/models/all`,
  start: `${API_BASE}/start`,
  stop: `${API_BASE}/stop`,
  config: `${API_BASE}/config`,
  mcp: `${API_BASE}/mcp`,
  providerModels: `${API_BASE}/providers/models`,
  providerManifests: `${API_BASE}/providers/manifests`,
  testConnection: `${API_BASE}/providers/test`,
  secretsKeys: `${API_BASE}/secrets/keys`,
  secretsTools: `${API_BASE}/secrets/tools`,
  restart: `${API_BASE}/system/restart`,
  factoryReset: `${API_BASE}/system/factory-reset`,
  clearRuntimeData: `${API_BASE}/system/clear-runtime-data`,
  wipeout: `${API_BASE}/system/wipeout`,
  hostSettings: `${API_BASE}/host`,
  terminalReset: `${API_BASE}/host/terminal/reset`,
  terminalSessions: `${API_BASE}/host/terminal/sessions`,
  processes: `${API_BASE}/runtime/processes`,
  connectorWebhook: (name: string) => `${API_BASE}/connectors/${encodeURIComponent(name)}/webhook`,
  activeRuns: (workspaceId: string) => `${API_BASE}/workspaces/${encodeURIComponent(workspaceId)}/active-runs`,
} as const

// Polling interval in milliseconds for state/metrics refresh
export const POLL_INTERVAL_MS = 10000

// Default log level
export const DEFAULT_LOG_LEVEL = 'INFO' as const

// Log level options
export const LOG_LEVELS = ['DEBUG', 'INFO', 'WARN', 'ERROR'] as const
export type LogLevel = (typeof LOG_LEVELS)[number]
