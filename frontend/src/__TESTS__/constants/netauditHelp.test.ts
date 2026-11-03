import { describe, expect, it } from 'vitest'
import { HELP_SECTIONS } from '../../constants/netaudit'

// The Safety part of Help must not say nothing leaves this computer while the Internet view can send the public address out.
describe('network help', () => {
  const safety = HELP_SECTIONS.safety.paragraphs.join('\n')

  it('says what the Internet view sends, to whom, and that it is off until switched on', () => {
    expect(safety).toContain('Internet view')
    expect(safety).toContain('public internet address')
    expect(safety).toContain('off until you turn it on')
  })

  it('says a look that could not be made is not the same as nothing being open', () => {
    expect(safety).toContain('does not mean nothing is open')
  })

  it('names the Router DNS check too: what it sends, to whom, that it only records, and that a missed comparison is not agreement', () => {
    expect(safety).toContain('Router DNS check')
    expect(safety).toContain('learns the names it asks and your public internet address')
    expect(safety).toContain('only keeps a record')
    expect(safety).toContain('does not mean the answers agree')
    expect(safety).not.toContain('Internet view is the one exception')
  })

  it('explains Nearby: what is sent, that it is off until switched on, that a device only claims what it is, and that silence is not proof', () => {
    const nearby = HELP_SECTIONS.nearby.paragraphs.join('\n')
    expect(nearby).toContain('two small questions')
    expect(nearby).toContain('off until you turn it on')
    expect(nearby).toContain('only a claim')
    expect(nearby).toContain('Silence is not proof')
    expect(nearby).toContain('Nothing is sent to the internet')
  })
})
