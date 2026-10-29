import { afterEach, describe, expect, it, vi } from 'vitest'
import { copyText } from '../../utils/clipboard'

// The browser clipboard API exists only on HTTPS and localhost. The admin UI is often served over plain HTTP on a LAN name,
// where navigator.clipboard is undefined: the copy must still work there, and say when it cannot.
afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

function stubExecCommand(result: boolean | Error) {
  const exec = vi.fn((command: string) => {
    if (result instanceof Error) throw result
    // What was selected when the command ran: the fallback copies the selection of a temporary box.
    const box = document.querySelector('textarea')
    seen.push(box ? box.value : '')
    return command === 'copy' && result
  })
  Object.defineProperty(document, 'execCommand', { value: exec, configurable: true, writable: true })
  return exec
}
const seen: string[] = []

describe('copyText', () => {
  it('uses the clipboard API where the page may', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    expect(await copyText('secret')).toBe(true)
    expect(writeText).toHaveBeenCalledWith('secret')
  })

  it('falls back to a selected box when there is no clipboard API (plain HTTP)', async () => {
    vi.stubGlobal('navigator', {})
    seen.length = 0
    const exec = stubExecCommand(true)
    expect(await copyText('lp_token')).toBe(true)
    expect(exec).toHaveBeenCalledWith('copy')
    expect(seen).toEqual(['lp_token'])
    expect(document.querySelector('textarea')).toBeNull() // the temporary box is removed
  })

  it('falls back when the API refuses', async () => {
    vi.stubGlobal('navigator', { clipboard: { writeText: vi.fn().mockRejectedValue(new Error('denied')) } })
    stubExecCommand(true)
    expect(await copyText('x')).toBe(true)
  })

  it('says it failed when nothing could copy, and leaves no box behind', async () => {
    vi.stubGlobal('navigator', {})
    stubExecCommand(false)
    expect(await copyText('x')).toBe(false)
    expect(document.querySelector('textarea')).toBeNull()
    stubExecCommand(new Error('not allowed'))
    expect(await copyText('x')).toBe(false)
    expect(document.querySelector('textarea')).toBeNull()
  })

  it('puts the focus back where it was', async () => {
    vi.stubGlobal('navigator', {})
    stubExecCommand(true)
    const input = document.createElement('input')
    document.body.appendChild(input)
    input.focus()
    await copyText('x')
    expect(document.activeElement).toBe(input)
  })
})
