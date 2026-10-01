import { describe, it, expect } from 'vitest'
import { documentTitle } from '../../utils/documentTitle'
import { APP_TITLE } from '../../config/brand'

describe('documentTitle', () => {
  it('names the page, and badges unread run notifications', () => {
    expect(documentTitle('Activity', 0)).toBe(`Activity · ${APP_TITLE}`)
    expect(documentTitle('Activity', 2)).toBe(`(2) Activity · ${APP_TITLE}`)
    expect(documentTitle(undefined, 0)).toBe(APP_TITLE)
  })
})
