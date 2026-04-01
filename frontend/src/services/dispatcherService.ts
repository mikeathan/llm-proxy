import type { Automation, DispatcherMetrics, TriggerResponse } from '../types/dispatcher'

const BASE_URL = '/admin/api/dispatcher'

export const DispatcherService = {
  async listAutomations(): Promise<Automation[]> {
    const res = await fetch(`${BASE_URL}/automations`)
    if (!res.ok) throw new Error('Failed to fetch automations')
    return res.json()
  },

  async triggerAutomation(workspace: string, automation: string): Promise<TriggerResponse> {
    const res = await fetch(`${BASE_URL}/trigger/${workspace}/${automation}`, {
      method: 'POST',
    })
    if (!res.ok) throw new Error('Failed to trigger automation')
    return res.json()
  },

  async getMetrics(): Promise<DispatcherMetrics> {
    const res = await fetch(`${BASE_URL}/metrics`)
    if (!res.ok) throw new Error('Failed to fetch dispatcher metrics')
    return res.json()
  },
}
