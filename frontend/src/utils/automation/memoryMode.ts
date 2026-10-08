import type { MemoryMode } from '../../types/automation'

/** What a memory override means in words; inherit names the global default it follows. */
export function memoryModeLabel(mode: MemoryMode | undefined, defaultOn: boolean): string {
  if (mode === 'on') return 'On'
  if (mode === 'off') return 'Off'
  return `Default (${defaultOn ? 'on' : 'off'})`
}
