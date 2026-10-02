import type { MemoryEntry, MemoryImportResult, MemoryInjectionPreview, MemoryPriority, MemoryView, NewMemory, NotesScope, OperatorNotes } from '../../types/memory'

// The backend answers errors as {"error": "…"}; show the reason, not the JSON.
async function failureReason(res: Response, what: string): Promise<Error> {
  const text = await res.text()
  try {
    const reason = (JSON.parse(text) as { error?: string }).error
    if (reason) return new Error(reason)
  } catch {
    // not JSON: fall through to the raw text
  }
  return new Error(`Failed to ${what}: ${res.status} - ${text}`)
}

export class MemoryService {
  // view 'hot' lists exactly the facts every run carries (workspace + user-wide),
  // from the injection query rather than the capped list; 'unused' lists the
  // facts never sent to a model and never returned by search.
  static async list(workspaceId: string, type?: string, view?: MemoryView): Promise<MemoryEntry[]> {
    const params = new URLSearchParams()
    if (type) params.set('type', type)
    if (view === 'hot') params.set('hot', 'true')
    if (view === 'unused') params.set('unused', 'true')
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

  // hot and priority are sent only when they are being changed; an edit of the
  // wording alone leaves the fact's place in the prompt as it was.
  static async update(
    workspaceId: string,
    id: number,
    title: string,
    content: string,
    change: { hot?: boolean; priority?: MemoryPriority } = {},
  ): Promise<void> {
    const body = { title, content, ...change }
    const res = await fetch(`/admin/api/memory/${workspaceId}/${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    if (!res.ok) throw await failureReason(res, 'update memory')
  }

  static async create(workspaceId: string, memory: NewMemory): Promise<MemoryEntry> {
    const res = await fetch(`/admin/api/memory/${workspaceId}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(memory),
    })
    if (!res.ok) throw await failureReason(res, 'save memory')
    return res.json()
  }

  // The operator's MEMORY.md: the global notes and this workspace's notes.
  static async getNotes(workspaceId: string): Promise<OperatorNotes> {
    const res = await fetch(`/admin/api/memory/${workspaceId}/notes`)
    if (!res.ok) throw await failureReason(res, 'load the operator notes')
    return res.json()
  }

  // Blank content removes the notes file.
  static async saveNotes(workspaceId: string, scope: NotesScope, content: string): Promise<void> {
    const res = await fetch(`/admin/api/memory/${workspaceId}/notes`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ scope, content }),
    })
    if (!res.ok) throw await failureReason(res, 'save the operator notes')
  }

  // The workspace's facts and the user-wide facts as an editable markdown file.
  static async exportMarkdown(workspaceId: string): Promise<string> {
    const res = await fetch(`/admin/api/memory/${workspaceId}/export`)
    if (!res.ok) throw await failureReason(res, 'export memory')
    return res.text()
  }

  // Adds the facts of a markdown file; duplicates are skipped and unusable
  // entries come back in issues, each with the line of its heading.
  static async importMarkdown(workspaceId: string, markdown: string): Promise<MemoryImportResult> {
    const res = await fetch(`/admin/api/memory/${workspaceId}/import`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ markdown }),
    })
    if (!res.ok) throw await failureReason(res, 'import memory')
    return res.json()
  }

  // model '' = no model chosen: the backend sizes with its fallback budget.
  static async injectionPreview(workspaceId: string, model: string): Promise<MemoryInjectionPreview> {
    const query = model ? `?model=${encodeURIComponent(model)}` : ''
    const res = await fetch(`/admin/api/memory/${workspaceId}/injection-preview${query}`)
    if (!res.ok) throw await failureReason(res, 'load the memory preview')
    return res.json()
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

  static async clearAll(workspaceId: string, type?: string): Promise<{deleted: number}> {
    const params = type ? `?type=${type}` : ''
    const res = await fetch(`/admin/api/memory/${workspaceId}${params}`, {
      method: 'DELETE',
    })
    if (!res.ok) {
      const text = await res.text()
      throw new Error(`Failed to clear memories: ${res.status} - ${text}`)
    }
    return res.json()
  }
}
