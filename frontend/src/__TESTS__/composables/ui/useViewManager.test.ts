import { describe, it, expect } from 'vitest'
import { ref } from 'vue'
import { useViewManager } from '../../../composables/ui/useViewManager'
import type { AutomationRun } from '../../../types/dispatcher'
import type { MemoryEntry } from '../../../types/memory'
import type { WorkspaceLocation } from '../../../types/routes'

// Workspace, file, section and conversation are route params (plan D18); the
// view manager derives the main pane from them plus the two selections that
// are not addressable (a run from the history list, a memory entry).
const EMPTY: WorkspaceLocation = { ws: null, filePath: '', section: null, assistant: false, conversationId: null }

function setup(at: Partial<WorkspaceLocation> = {}) {
  const deps = {
    location: ref<WorkspaceLocation>({ ...EMPTY, ...at }),
    selectedRun: ref<AutomationRun | null>(null),
    selectedMemory: ref<MemoryEntry | null>(null),
    isMobile: ref(false),
  }
  return { deps, vm: useViewManager(deps) }
}

describe('useViewManager', () => {
  it.each<[Partial<WorkspaceLocation>, string]>([
    [{}, 'overview'],
    [{ ws: 'ws' }, 'overview'],
    [{ ws: 'ws', filePath: 'a.md' }, 'editor'],
    [{ ws: 'ws', assistant: true }, 'assistant'],
    [{ ws: 'ws', section: 'memory' }, 'memory'],
    [{ ws: 'ws', section: 'settings' }, 'settings'],
    [{ ws: 'ws', section: 'playbooks' }, 'playbooks'],
    [{ ws: 'ws', section: 'heartbeat' }, 'heartbeat'],
  ])('%o → %s', (at, view) => {
    expect(setup(at).vm.activeMainView.value).toBe(view)
  })

  it('shows a selected run over the addressed page', () => {
    const { deps, vm } = setup({ ws: 'ws', filePath: 'a.md' })
    deps.selectedRun.value = { id: 'r' } as AutomationRun
    expect(vm.activeMainView.value).toBe('history')
  })

  it('opens a memory entry only within the memory section', () => {
    const { deps, vm } = setup({ ws: 'ws', section: 'memory' })
    deps.selectedMemory.value = {} as MemoryEntry
    expect(vm.activeMainView.value).toBe('memory-detail')
    deps.location.value = { ...EMPTY, ws: 'ws', filePath: 'a.md' }
    expect(vm.activeMainView.value).toBe('editor')
  })

  it('shows one pane below lg: the explorer on the overview, the page otherwise', () => {
    const { deps, vm } = setup({ ws: 'ws' })
    expect([vm.explorerVisible.value, vm.mainVisible.value]).toEqual([true, true])
    deps.isMobile.value = true
    expect([vm.explorerVisible.value, vm.mainVisible.value]).toEqual([true, false])
    deps.location.value = { ...EMPTY, ws: 'ws', filePath: 'a.md' }
    expect([vm.explorerVisible.value, vm.mainVisible.value]).toEqual([false, true])
  })
})
