import type { Ref } from 'vue'
import { useWorkspaceConfigField } from '../automation/useWorkspaceConfigField'
import type { MemoryMode } from '../../types/automation'

const KNOWN_MODES: MemoryMode[] = ['', 'on', 'off']

const normalizeMode = (raw: unknown): MemoryMode => (KNOWN_MODES.includes(raw as MemoryMode) ? (raw as MemoryMode) : '')

// A workspace's assistant memory override.
export function useAssistantMemory(workspaceId: Readonly<Ref<string>>) {
  const { value: mode, loading, saving, error, save } = useWorkspaceConfigField<MemoryMode>(workspaceId, 'assistant_memory', normalizeMode, '')
  return { mode, loading, saving, error, save }
}
