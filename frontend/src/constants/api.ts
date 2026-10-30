// API endpoint constants for the admin panel
const API_BASE = '/admin/api'

export const API_ENDPOINTS = {
  state: `${API_BASE}/state`,
  version: `${API_BASE}/version`,
  metrics: `${API_BASE}/metrics`,
  logs: `${API_BASE}/logs`,
  appLogs: `${API_BASE}/app-logs/tail`,
  logLevel: `${API_BASE}/log-level`,
  models: `${API_BASE}/models`,
  profiles: `${API_BASE}/profiles`,
  profileSchema: `${API_BASE}/profile-schema`,
  helpers: `${API_BASE}/helpers`,
  netauditScope: `${API_BASE}/netaudit/scope`,
  netauditReadiness: `${API_BASE}/netaudit/readiness`,
  netauditFindings: `${API_BASE}/netaudit/findings`,
  netauditTargetsCheck: `${API_BASE}/netaudit/targets/check`,
  netauditAcks: `${API_BASE}/netaudit/acks`,
  netauditFixes: `${API_BASE}/netaudit/fixes`,
  netauditExpectations: `${API_BASE}/netaudit/expectations`,
  netauditDevices: `${API_BASE}/netaudit/devices`,
  netauditGatewayConfirm: `${API_BASE}/netaudit/devices/gateway/confirm`,
  netauditPresence: `${API_BASE}/netaudit/presence`,
  netauditDecoys: `${API_BASE}/netaudit/decoys`,
  netauditDeep: `${API_BASE}/netaudit/deep`,
  authStatus: `${API_BASE}/auth/status`,
  authSetup: `${API_BASE}/auth/setup`,
  authLogin: `${API_BASE}/auth/login`,
  authLogout: `${API_BASE}/auth/logout`,
  authReauth: `${API_BASE}/auth/reauth`,
  authSessions: `${API_BASE}/auth/sessions`,
  authSessionsRevokeAll: `${API_BASE}/auth/sessions/revoke-all`,
  authPassphrase: `${API_BASE}/auth/passphrase`,
  authTokens: `${API_BASE}/auth/tokens`,
  authEvents: `${API_BASE}/auth/events`,
  authExposure: `${API_BASE}/auth/exposure`,
  authSessionLength: `${API_BASE}/auth/session-length`,
  netauditVulnData: `${API_BASE}/netaudit/vulndata`,
  netauditVulnDownload: `${API_BASE}/netaudit/vulndata/download`,
  netauditVulnCancel: `${API_BASE}/netaudit/vulndata/cancel`,
  netauditVulnImport: `${API_BASE}/netaudit/vulndata/import`,
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
  activeRunsGlobal: `${API_BASE}/active-runs`,
  // Operator actions on a queued inbound caller (external /v1 request waiting
  // for the local model): serve it now, or drop it.
  queuePromote: (key: string) => `${API_BASE}/queue/${encodeURIComponent(key)}/promote`,
  queueCancel: (key: string) => `${API_BASE}/queue/${encodeURIComponent(key)}/cancel`,
} as const

// Polling interval in milliseconds for state/metrics refresh
export const POLL_INTERVAL_MS = 10000

// Default log level
export const DEFAULT_LOG_LEVEL = 'INFO' as const

// Log level options
export const LOG_LEVELS = ['DEBUG', 'INFO', 'WARN', 'ERROR'] as const
