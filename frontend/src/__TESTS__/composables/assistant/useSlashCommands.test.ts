import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { ref } from 'vue'

const service = vi.hoisted(() => ({ list: vi.fn(), preview: vi.fn() }))
vi.mock('../../../services/commands/commandService', () => ({ CommandService: service }))

import { useSlashCommands, PREVIEW_DEBOUNCE_MS } from '../../../composables/assistant/useSlashCommands'

const remind = { name: 'remind', usage: '/remind <when> <message>', summary: 'Set a reminder', examples: ['tomorrow 9am call'] }

beforeEach(() => {
  vi.useFakeTimers()
  service.list.mockReset().mockResolvedValue([remind])
  service.preview.mockReset().mockResolvedValue({ matched: true, command: 'remind', text: 'Tue 13 Oct 2026 09:00' })
})
afterEach(() => vi.useRealTimers())

async function type(text: ReturnType<typeof ref<string>>, value: string) {
  text.value = value
  await flushPromises()
}

describe('useSlashCommands', () => {
  it('offers matching commands while only the name is typed', async () => {
    const text = ref('')
    const { suggestions } = useSlashCommands(text)
    await type(text, '/')
    expect(suggestions.value.map(c => c.name)).toEqual(['remind'])
    await type(text, '/re')
    expect(suggestions.value).toHaveLength(1)
    await type(text, '/zz')
    expect(suggestions.value).toEqual([])
  })

  it('does not load the commands until a slash is typed', async () => {
    const text = ref('')
    useSlashCommands(text)
    await type(text, 'hello')
    expect(service.list).not.toHaveBeenCalled()
  })

  it('knows the active command once it is followed by a space', async () => {
    const text = ref('')
    const { active, suggestions } = useSlashCommands(text)
    await type(text, '/remind ')
    expect(active.value?.name).toBe('remind')
    expect(suggestions.value).toEqual([])
  })

  it('previews after a pause in typing, once', async () => {
    const text = ref('')
    const { preview } = useSlashCommands(text)
    await type(text, '/remind t')
    await type(text, '/remind tomorrow 9am')
    expect(service.preview).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(PREVIEW_DEBOUNCE_MS)
    expect(service.preview).toHaveBeenCalledTimes(1)
    expect(service.preview).toHaveBeenCalledWith('/remind tomorrow 9am')
    expect(preview.value).toEqual({ text: 'Tue 13 Oct 2026 09:00', error: '' })
  })

  it('shows why a command would be refused', async () => {
    service.preview.mockResolvedValue({ matched: true, command: 'remind', error: 'couldn\'t read "blah"' })
    const text = ref('')
    const { preview } = useSlashCommands(text)
    await type(text, '/remind blah')
    await vi.advanceTimersByTimeAsync(PREVIEW_DEBOUNCE_MS)
    expect(preview.value?.error).toContain('blah')
  })

  it('drops a preview that arrives after newer text', async () => {
    let release: (v: unknown) => void = () => {}
    service.preview.mockImplementationOnce(() => new Promise(r => { release = r }))
    const text = ref('')
    const { preview } = useSlashCommands(text)
    await type(text, '/remind a')
    await vi.advanceTimersByTimeAsync(PREVIEW_DEBOUNCE_MS)
    await type(text, 'plain text now')
    release({ matched: true, text: 'stale' })
    await flushPromises()
    expect(preview.value).toBeNull()
  })

  it('completes a command and fills an example', async () => {
    const text = ref('')
    const { completion, withExample } = useSlashCommands(text)
    expect(completion(remind)).toBe('/remind ')
    expect(withExample(remind, 'in 2h tea')).toBe('/remind in 2h tea')
  })
})
