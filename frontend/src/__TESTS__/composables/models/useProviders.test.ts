import { describe, expect, it } from 'vitest'
import { useProviders } from '../../../composables/models/useProviders'
import { PROVIDER_IDS } from '../../../constants/providers'

describe('useProviders', () => {
  it('lists every cloud provider and never the local engine, which is not a provider of keys', () => {
    const cloud = useProviders().cloudProviders.value
    expect(cloud).toEqual(PROVIDER_IDS.filter((id) => id !== 'local'))
    expect(useProviders().allProviders.value[0]).toBe('local')
  })

  it('names a provider by its label when no manifest names it', () => {
    expect(useProviders().getLabel('nvidia')).toBe('NVIDIA NIM')
    expect(useProviders().getLabel('someone-new')).toBe('someone-new')
  })
})
