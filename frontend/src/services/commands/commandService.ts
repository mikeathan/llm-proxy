import type { CommandInfo, CommandPreview, CommandRunResult, CommandTurn } from '../../types/commands'
import { browserTimeZone } from '../../utils/format/timezone'

async function post<T>(path: string, message: string, workspaceId?: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(`/admin/api/commands/${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    signal,
    // The browser's zone is the fallback when the owner has not set one.
    // The chat's workspace is where /run starts an automation.
    body: JSON.stringify({ message, timezone: browserTimeZone(), ...(workspaceId ? { workspace_id: workspaceId } : {}) }),
  })
  if (!res.ok) throw new Error(`Command ${path} failed: ${res.status} - ${await res.text()}`)
  return res.json()
}

export class CommandService {
  static async list(): Promise<CommandInfo[]> {
    const res = await fetch('/admin/api/commands')
    if (!res.ok) throw new Error(`Failed to list commands: ${res.status} - ${await res.text()}`)
    return ((await res.json()) as { commands: CommandInfo[] }).commands
  }

  static async run(message: string, workspaceId?: string): Promise<CommandRunResult> {
    const body = await post<{ handled: boolean; reply?: string; failed?: boolean; turn?: CommandTurn }>('run', message, workspaceId)
    return { handled: body.handled, reply: body.reply ?? '', failed: body.failed ?? false, ...(body.turn && { turn: body.turn }) }
  }

  static preview(message: string, signal?: AbortSignal, workspaceId?: string): Promise<CommandPreview> {
    return post<CommandPreview>('preview', message, workspaceId, signal)
  }
}
