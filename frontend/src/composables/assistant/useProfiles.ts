import { ref } from 'vue'
import { ProfileService } from '../../services/profile/profileService'
import type { PromptProfile } from '../../types/profile'

const LOAD_FAILED = 'The agents could not be read.'

// The prompt-profile catalog, shared by every picker. It is read once and kept; refresh() reads it again (a profile
// file was added or edited on the server).
const profiles = ref<PromptProfile[]>([])
const error = ref<string | null>(null)
let inFlight: Promise<void> | null = null

function read(): Promise<void> {
  inFlight ??= ProfileService.list()
    .then((list) => {
      profiles.value = list
      error.value = null
    })
    .catch((err: unknown) => {
      error.value = err instanceof Error ? err.message : LOAD_FAILED
    })
    .finally(() => {
      inFlight = null
    })
  return inFlight
}

export function useProfiles() {
  if (profiles.value.length === 0) void read()
  return { profiles, error, refresh: read }
}
