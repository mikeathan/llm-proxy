import { describe, expect, it } from 'vitest'
import { plateKind } from '../../../utils/newsroom/plate'

describe('plateKind', () => {
  it('reads the category before the name', () => {
    expect(plateKind('Technology', 'Morning things')).toBe('tech')
    expect(plateKind('World', 'Tech and markets')).toBe('world')
  })
  it('falls back to the name, then to a plain paper plate', () => {
    expect(plateKind('', 'UK politics')).toBe('politics')
    expect(plateKind(undefined, 'Gardening')).toBe('paper')
  })
})
