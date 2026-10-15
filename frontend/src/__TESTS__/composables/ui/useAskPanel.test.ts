import { afterEach, describe, expect, it } from 'vitest'
import { useAskPanel } from '../../../composables/ui/useAskPanel'

describe('useAskPanel', () => {
  afterEach(() => useAskPanel().hide())

  it('starts closed and opens, closes and toggles', () => {
    const panel = useAskPanel()
    expect(panel.open.value).toBe(false)
    panel.show()
    expect(panel.open.value).toBe(true)
    panel.hide()
    expect(panel.open.value).toBe(false)
    panel.toggle()
    expect(panel.open.value).toBe(true)
    panel.toggle()
    expect(panel.open.value).toBe(false)
  })

  it('is one panel for everything that asks about it', () => {
    useAskPanel().show()
    expect(useAskPanel().open.value).toBe(true)
  })
})
