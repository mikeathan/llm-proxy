import { describe, expect, it } from 'vitest'
import { useNetauditHelp } from '../../../composables/netaudit/useNetauditHelp'

describe('useNetauditHelp', () => {
  it('opens on the asked topic and closes', () => {
    const help = useNetauditHelp()
    help.show('safety')
    expect(help.open.value).toBe(true)
    expect(help.topic.value).toBe('safety')
    help.hide()
    expect(help.open.value).toBe(false)
    help.show()
    expect(help.topic.value).toBe('basics')
  })
})
