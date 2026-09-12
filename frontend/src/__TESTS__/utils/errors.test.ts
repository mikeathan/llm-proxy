import { describe, expect, it } from 'vitest'
import { errorMessage, UNKNOWN_ERROR_MESSAGE } from '../../utils/errors'

describe('errorMessage', () => {
  it('returns the message of an Error', () => {
    expect(errorMessage(new Error('boom'))).toBe('boom')
  })

  it('falls back for an Error without a message', () => {
    expect(errorMessage(new Error(''), 'fallback')).toBe('fallback')
  })

  it('accepts a plain string', () => {
    expect(errorMessage('direct')).toBe('direct')
  })

  it('returns the fallback for non-error input', () => {
    expect(errorMessage(42)).toBe(UNKNOWN_ERROR_MESSAGE)
    expect(errorMessage(null, 'custom')).toBe('custom')
    expect(errorMessage({ nested: true })).toBe(UNKNOWN_ERROR_MESSAGE)
  })
})
