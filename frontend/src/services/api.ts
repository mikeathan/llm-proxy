import type { AdminState, McpServer, ProcessLogs, SystemMetrics } from '../types'

export const ApiService = {
  async fetchState(): Promise<AdminState> {
    const res = await fetch('/admin/api/state?available=1')
    if (!res.ok) throw new Error(await res.text())
    return (await res.json()) as AdminState
  },

  async fetchMetrics(): Promise<SystemMetrics> {
    const res = await fetch('/admin/api/metrics')
    if (!res.ok) throw new Error(await res.text())
    return (await res.json()) as SystemMetrics
  },

  async fetchLogs(): Promise<ProcessLogs> {
    const res = await fetch('/admin/api/logs')
    if (!res.ok) throw new Error(await res.text())
    return (await res.json()) as ProcessLogs
  },

  async fetchMCPServers(): Promise<McpServer[]> {
    const res = await fetch('/admin/api/mcp')
    if (!res.ok) throw new Error(await res.text())
    const data = await res.json()
    return Array.isArray(data) ? data : []
  },

  async fetchLogLevel(): Promise<string> {
    const res = await fetch('/admin/api/log-level')
    if (!res.ok) throw new Error(await res.text())
    const data = await res.json()
    return data.level || 'INFO'
  },

  async updateLogLevel(level: string): Promise<void> {
    const res = await fetch('/admin/api/log-level', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ level })
    })
    if (!res.ok) throw new Error(await res.text())
  },

  async startModel(name: string): Promise<void> {
    const res = await fetch('/admin/api/start', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name })
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({}))
      throw new Error(err.error || res.statusText)
    }
  },

  async stopModel(): Promise<void> {
    const res = await fetch('/admin/api/stop', { method: 'POST' })
    if (!res.ok) {
      const err = await res.json().catch(() => ({}))
      throw new Error(err.error || res.statusText)
    }
  },

  async addModel(payload: any): Promise<void> {
    const res = await fetch('/admin/api/models', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({}))
      throw new Error(err.error || res.statusText)
    }
  },

  async removeModel(name: string): Promise<void> {
    const res = await fetch(`/admin/api/models?name=${encodeURIComponent(name)}`, { method: 'DELETE' })
    if (!res.ok) {
      const err = await res.json().catch(() => ({}))
      throw new Error(err.error || res.statusText)
    }
  },

  async updateModel(payload: any): Promise<void> {
    const res = await fetch('/admin/api/models', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({}))
      throw new Error(err.error || res.statusText)
    }
  },

  async updateConfig(payload: any): Promise<void> {
    const res = await fetch('/admin/api/config', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({}))
      throw new Error(err.error || res.statusText)
    }
  },

  async addMCPServer(payload: any): Promise<void> {
    const res = await fetch('/admin/api/mcp', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({}))
      throw new Error(err.error || res.statusText)
    }
  },

  async removeMCPServer(name: string): Promise<void> {
    const res = await fetch(`/admin/api/mcp?name=${encodeURIComponent(name)}`, { method: 'DELETE' })
    if (!res.ok) {
      const err = await res.json().catch(() => ({}))
      throw new Error(err.error || res.statusText)
    }
  },

  async toggleMCPServer(server: any): Promise<void> {
    const res = await fetch('/admin/api/mcp', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ...server, enabled: !server.enabled })
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({}))
      throw new Error(err.error || res.statusText)
    }
  },

  async submitGuardrailDecision(decisionId: string, allow: boolean, persist: boolean): Promise<void> {
    const res = await fetch('/admin/api/conversation/guardrail-decision', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ decision_id: decisionId, allow, persist })
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({}))
      throw new Error(err.error || res.statusText)
    }
  }
}
