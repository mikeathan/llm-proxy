import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { ref } from 'vue'
import { memoryStorage } from '../../helpers/memoryStorage'

const workspaces = ref<{ id: string }[]>([])
const fetchWorkspaces = vi.fn()
vi.mock('../../../composables/automation/useDispatcher', () => ({ useDispatcher: () => ({ workspaces, fetchWorkspaces }) }))

import { useActiveWorkspace } from '../../../composables/ui/useActiveWorkspace'
import { useLastWorkspace } from '../../../composables/ui/useLastWorkspace'

describe('useActiveWorkspace', () => {
  beforeEach(() => {
    vi.stubGlobal('localStorage', memoryStorage())
    workspaces.value = []
    fetchWorkspaces.mockReset().mockImplementation(async () => {
      workspaces.value = [{ id: 'alpha' }, { id: 'beta' }]
    })
    useLastWorkspace().forget('alpha')
    useLastWorkspace().forget('beta')
    useLastWorkspace().forget('gone')
  })
  afterEach(() => vi.unstubAllGlobals())

  it('loads the workspaces once and defaults to the first when none was remembered', async () => {
    const { refresh, active } = useActiveWorkspace()
    expect(active.value).toBeNull()
    await refresh()
    expect(fetchWorkspaces).toHaveBeenCalledTimes(1)
    expect(active.value).toBe('alpha')
  })

  it('keeps the workspace last used while it still exists', async () => {
    useLastWorkspace().remember('beta')
    const { refresh, active } = useActiveWorkspace()
    await refresh()
    expect(active.value).toBe('beta')
  })

  it('forgets a remembered workspace that no longer exists, falling back to the first', async () => {
    useLastWorkspace().remember('gone')
    const { refresh, active } = useActiveWorkspace()
    await refresh()
    expect(active.value).toBe('alpha')
  })

  it('does not forget anything before the list has loaded', async () => {
    useLastWorkspace().remember('beta')
    const { active } = useActiveWorkspace()
    await flushPromises()
    expect(active.value).toBe('beta')
  })

  it('moves on when the active workspace is deleted elsewhere', async () => {
    const { refresh, select, active } = useActiveWorkspace()
    await refresh()
    select('beta')
    workspaces.value = [{ id: 'alpha' }]
    await flushPromises()
    expect(active.value).toBe('alpha')
  })

  it('selects a workspace, and ignores a name that is not one', async () => {
    const { refresh, select, active } = useActiveWorkspace()
    await refresh()
    select('beta')
    expect(active.value).toBe('beta')
    select('nope')
    expect(active.value).toBe('beta')
  })

  it('has nothing to default to without workspaces', async () => {
    fetchWorkspaces.mockImplementation(async () => {
      workspaces.value = []
    })
    const { refresh, active } = useActiveWorkspace()
    await refresh()
    expect(active.value).toBeNull()
  })
})
