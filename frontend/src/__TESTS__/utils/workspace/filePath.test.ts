import { describe, it, expect } from 'vitest'
import { encodeFilePath, isSafeRelativePath } from '../../../utils/workspace/filePath'

describe('encodeFilePath', () => {
  it.each([
    ['plan.md', 'plan.md'],
    ['docs/plan.md', 'docs/plan.md'],
    ['my notes/a b.md', 'my%20notes/a%20b.md'],
    ['issue #4.md', 'issue%20%234.md'],
    ['100%/done?.md', '100%25/done%3F.md'],
    ['café/naïve.md', 'caf%C3%A9/na%C3%AFve.md'],
  ])('%s → %s', (path, encoded) => {
    expect(encodeFilePath(path)).toBe(encoded)
  })
})

describe('isSafeRelativePath', () => {
  it.each([
    ['plan.md', true],
    ['docs/deep/notes.md', true],
    ['v1..2.md', true],
    ['', false],
    ['/etc/passwd', false],
    ['../escape.md', false],
    ['docs/../../x', false],
    ['docs//x.md', false],
    ['./x.md', false],
    ['docs\\x.md', false],
  ])('%s → %s', (path, safe) => {
    expect(isSafeRelativePath(path)).toBe(safe)
  })
})
