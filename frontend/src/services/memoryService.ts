import type { MemoryEntry } from '../types/memory'

export class MemoryService {
  static async list(workspaceId: string, type?: string): Promise<MemoryEntry[]> {
    const params = new URLSearchParams()
    if (type) params.set('type', type)
    const query = params.toString() ? `?${params.toString()}` : ''
    const res = await fetch(`/admin/api/memory/${workspaceId}${query}`)
    if (!res.ok) {
      const text = await res.text()
      throw new Error(`Failed to list memories: ${res.status} - ${text}`)
    }
    return res.json()
  }

  static async search(workspaceId: string, query: string, limit?: number): Promise<MemoryEntry[]> {
    const res = await fetch(`/admin/api/memory/${workspaceId}/search`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ query, limit: limit || 20 }),
    })
    if (!res.ok) {
      const text = await res.text()
      throw new Error(`Failed to search memories: ${res.status} - ${text}`)
    }
    return res.json()
  }

  static async update(workspaceId: string, id: number, title: string, content: string): Promise<void> {
    const res = await fetch(`/admin/api/memory/${workspaceId}/${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ title, content }),
    })
    if (!res.ok) {
      const text = await res.text()
      throw new Error(`Failed to update memory: ${res.status} - ${text}`)
    }
  }

  static async delete(workspaceId: string, id: number): Promise<void> {
    const res = await fetch(`/admin/api/memory/${workspaceId}/${id}`, {
      method: 'DELETE',
    })
    if (!res.ok) {
      const text = await res.text()
      throw new Error(`Failed to delete memory: ${res.status} - ${text}`)
    }
  }
}
