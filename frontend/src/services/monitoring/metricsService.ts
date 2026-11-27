import { API_ENDPOINTS } from '../../constants/api'
import type { SystemMetrics, ProcessLogs, ServiceFootprint } from '../../types/metrics'
import type { LogLevel } from '../../types/api'
import { get, put, del } from '../httpClient'

export const MetricsApiService = {
  fetchMetrics: (): Promise<SystemMetrics> =>
    get<SystemMetrics>(API_ENDPOINTS.metrics),

  fetchFootprint: (): Promise<ServiceFootprint> =>
    get<ServiceFootprint>(API_ENDPOINTS.metricsFootprint),

  fetchLogs: (): Promise<ProcessLogs> =>
    get<ProcessLogs>(API_ENDPOINTS.logs),

  fetchAppLogs: (): Promise<{ logs: string }> =>
    get<{ logs: string }>(API_ENDPOINTS.appLogs),

  clearLogs: (): Promise<void> =>
    del<void>(API_ENDPOINTS.logs),

  clearAppLogs: (): Promise<void> =>
    del<void>(API_ENDPOINTS.appLogs.replace('/tail', '')),

  fetchLogLevel: async (): Promise<LogLevel> => {
    const data = await get<{ level: LogLevel }>(API_ENDPOINTS.logLevel)
    return data.level
  },

  updateLogLevel: (level: LogLevel): Promise<void> =>
    put<void>(API_ENDPOINTS.logLevel, { level }),
}
