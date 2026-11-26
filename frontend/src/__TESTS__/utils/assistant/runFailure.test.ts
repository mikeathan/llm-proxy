import { describe, expect, it } from 'vitest'
import { describeRunFailure } from '../../../utils/assistant/runFailure'

describe('describeRunFailure', () => {
  it('names the fix for the errors seen on vertex', () => {
    const opencode = describeRunFailure('The model server rejected the request (HTTP 400): Request is missing x-opencode-session and cannot be routed efficiently.')
    expect(opencode.title).toBe("The model's provider turned the request down.")
    expect(opencode.fixInModels).toBe(true)

    const window = describeRunFailure("The assistant run failed: prompt too large for the model's context window: the system prompt, task and tools leave no room")
    expect(window.title).toMatch(/no longer fits/)
    expect(window.fixInModels).toBe(false)
  })

  it('falls back to a plain sentence for anything else', () => {
    expect(describeRunFailure('something odd').title).toBe('The run failed.')
  })
})
