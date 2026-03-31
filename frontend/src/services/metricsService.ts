import { API_ENDPOINTS } from '../constants/api'
import type { SystemMetrics, ProcessLogs } from '../types/metrics'
import type { LogLevel } from '../constants/api'

async function handleResponse<T>(res: Response): Promise<T> {
  if (!res.ok) {
    const err = await res.json().catch(() => ({})) as Record<string, string>
    throw new Error(err['error'] || res.statusText)
  }
  return res.json() as Promise<T>
}

export const MetricsApiService = {
  fetchMetrics: (): Promise<SystemMetrics> =>
    fetch(API_ENDPOINTS.metrics).then(r => handleResponse<SystemMetrics>(r)),

  fetchLogs: (): Promise<ProcessLogs> =>
    fetch(API_ENDPOINTS.logs).then(r => handleResponse<ProcessLogs>(r)),

  fetchAppLogs: (): Promise<{ logs: string }> =>
    fetch(API_ENDPOINTS.appLogs).then(r => handleResponse<{ logs: string }>(r)),

  clearLogs: (): Promise<void> =>
    fetch(API_ENDPOINTS.logs, { method: 'DELETE' }).then(r => handleResponse<void>(r)),

  clearAppLogs: (): Promise<void> =>
    fetch(API_ENDPOINTS.appLogs.replace('/tail', ''), { method: 'DELETE' }).then(r => handleResponse<void>(r)),

  fetchLogLevel: async (): Promise<LogLevel> => {
    const data = await fetch(API_ENDPOINTS.logLevel).then(r => handleResponse<{ level: LogLevel }>(r))
    return data.level
  },

  updateLogLevel: async (level: LogLevel): Promise<void> => {
    const res = await fetch(API_ENDPOINTS.logLevel, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ level }),
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({})) as Record<string, string>
      throw new Error(err['error'] || res.statusText)
    }
  },
}
