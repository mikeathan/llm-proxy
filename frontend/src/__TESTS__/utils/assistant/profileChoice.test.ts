import { describe, it, expect } from 'vitest'
import { defaultProfile, profileLabel, profileOptions } from '../../../utils/assistant/profileChoice'
import type { PromptProfile } from '../../../types/profile'

const profile = (id: string, name: string, default_for: PromptProfile['default_for'] = []): PromptProfile => ({
  id, name, summary: `${name} summary`, source: 'builtin', answer_first: false, may_ask: false, memory_save: 'model', reminders: false, default_for,
})
const PROFILES = [profile('builder', 'Builder'), profile('general', 'General', ['assistant']), profile('task-runner', 'Task runner', ['automation'])]

describe('defaultProfile', () => {
  it("is the kind's default when the workspace chose nothing", () => {
    expect(defaultProfile(PROFILES, 'assistant')?.id).toBe('general')
    expect(defaultProfile(PROFILES, 'automation')?.id).toBe('task-runner')
  })

  it("is the workspace's choice when that profile exists", () => {
    expect(defaultProfile(PROFILES, 'assistant', 'builder')?.id).toBe('builder')
  })

  it('ignores a workspace choice that no longer exists', () => {
    expect(defaultProfile(PROFILES, 'assistant', 'deleted')?.id).toBe('general')
  })

  it('is undefined until the catalog has loaded', () => {
    expect(defaultProfile([], 'assistant')).toBeUndefined()
  })
})

describe('profileOptions', () => {
  it('lists every profile and marks the one a chat starts with', () => {
    expect(profileOptions(PROFILES, 'assistant')).toEqual([
      { value: 'builder', label: 'Builder' },
      { value: 'general', label: 'General (default)' },
      { value: 'task-runner', label: 'Task runner' },
    ])
  })

  it("says so when the start is the workspace's choice", () => {
    const labels = profileOptions(PROFILES, 'assistant', 'builder').map((o) => o.label)
    expect(labels).toEqual(['Builder (this workspace)', 'General', 'Task runner'])
  })

  it('marks nothing while the catalog is empty', () => {
    expect(profileOptions([], 'assistant')).toEqual([])
  })
})

describe('profileLabel', () => {
  it('names a chosen profile', () => {
    expect(profileLabel(PROFILES, 'automation', 'builder')).toBe('Builder')
  })

  it("says which profile 'none chosen' means", () => {
    expect(profileLabel(PROFILES, 'automation')).toBe('Default (Task runner)')
    expect(profileLabel(PROFILES, 'assistant', '')).toBe('Default (General)')
  })

  it('shows the id of a profile that no longer exists, and plain Default before the catalog loads', () => {
    expect(profileLabel(PROFILES, 'automation', 'deleted')).toBe('deleted')
    expect(profileLabel([], 'automation')).toBe('Default')
  })
})
