import type { Ref } from 'vue'
import { useWorkspaceConfigField } from '../automation/useWorkspaceConfigField'

const normalizeProfile = (raw: unknown): string => (typeof raw === 'string' ? raw : '')

// The profile this workspace's chats start with ('' = the default for chats).
export function useWorkspaceProfile(workspaceId: Readonly<Ref<string>>) {
  const { value: profile, loading, saving, error, save } = useWorkspaceConfigField<string>(workspaceId, 'assistant_profile', normalizeProfile, '')
  return { profile, loading, saving, error, save }
}
