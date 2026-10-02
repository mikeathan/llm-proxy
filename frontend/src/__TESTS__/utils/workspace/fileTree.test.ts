import { describe, it, expect } from 'vitest'
import { buildFileTree, filterTreeEntries, hasSelectionWithin, isCoveredBySelection, isPathWithin, topLevelPaths, treeFilePaths, withoutNestedPaths } from '../../../utils/workspace/fileTree'
import type { FileTreeNode, TreeEntry } from '../../../types/workspace'

const file = (path: string): TreeEntry => ({ path, type: 'file' })
const dir = (path: string, collapsed = false): TreeEntry => ({ path, type: 'dir', ...(collapsed ? { collapsed } : {}) })
const names = (nodes: FileTreeNode[]) => nodes.map((n) => n.name)

describe('buildFileTree', () => {
  it('returns no nodes for an empty tree', () => {
    expect(buildFileTree([])).toEqual([])
  })

  it('renders a workspace with only root files', () => {
    expect(names(buildFileTree([file('b.md'), file('a.md')]))).toEqual(['a.md', 'b.md'])
  })

  it('nests entries and orders directories first, then by name', () => {
    const nodes = buildFileTree([file('z.md'), dir('docs'), file('docs/b.md'), dir('docs/deep'), file('docs/a.md'), file('a.md'), dir('b')])
    expect(names(nodes)).toEqual(['b', 'docs', 'a.md', 'z.md'])
    const docs = nodes[1]!
    expect(names(docs.children)).toEqual(['deep', 'a.md', 'b.md'])
    expect(docs.children[1]!.path).toBe('docs/a.md')
  })

  it('keeps empty directories and marks heavy ones collapsed', () => {
    const nodes = buildFileTree([dir('empty'), dir('node_modules', true)])
    expect(nodes.map((n) => [n.name, n.type, n.collapsed, n.children.length])).toEqual([
      ['empty', 'dir', false, 0],
      ['node_modules', 'dir', true, 0],
    ])
  })

  it('handles deep paths and the same name at different levels', () => {
    const nodes = buildFileTree([dir('a'), dir('a/a'), dir('a/a/a'), file('a/a/a/a.md'), file('a/a.md')])
    expect(nodes[0]!.children[0]!.children[0]!.children[0]!.path).toBe('a/a/a/a.md')
    expect(names(nodes[0]!.children)).toEqual(['a', 'a.md'])
  })

  it('creates missing parents so a truncated listing still nests', () => {
    const nodes = buildFileTree([file('x/y/z.md')])
    expect(nodes[0]!.path).toBe('x')
    expect(nodes[0]!.children[0]!.children[0]!.path).toBe('x/y/z.md')
  })
})

describe('treeFilePaths', () => {
  it('lists file paths only, sorted by path', () => {
    expect(treeFilePaths({ entries: [dir('docs'), file('docs/a.md'), file('b.md')], truncated: false })).toEqual(['b.md', 'docs/a.md'])
  })
})

// Measured (Phase 6): at the 5000-entry cap a filter that expands every folder
// rendered ~4900 rows — a 1 s main-thread block at 4x CPU throttling. The
// filter keeps at most `limit` matching files and reports the full count.
describe('filterTreeEntries', () => {
  const entries: TreeEntry[] = [
    { path: 'docs', type: 'dir' },
    { path: 'docs/Notes.md', type: 'file' },
    { path: 'docs/plan.md', type: 'file' },
    { path: 'notes-old.md', type: 'file' },
    { path: 'src/app.ts', type: 'file' },
  ]

  it('keeps every entry when there is no query', () => {
    expect(filterTreeEntries(entries, '  ', 10)).toEqual({ entries, total: entries.length })
  })

  it('keeps matching files only, ignoring case, with the full match count', () => {
    const { entries: kept, total } = filterTreeEntries(entries, 'NOTES', 10)
    expect(kept.map((e) => e.path)).toEqual(['docs/Notes.md', 'notes-old.md'])
    expect(total).toBe(2)
  })

  it('stops at the limit but still counts every match', () => {
    const { entries: kept, total } = filterTreeEntries(entries, '.', 2)
    expect(kept).toHaveLength(2)
    expect(total).toBe(4)
  })
})

describe('deletion helpers', () => {
  it('drops a path whose folder is also listed, keeping the first-listed order', () => {
    expect(withoutNestedPaths(['docs/deep/notes.md', 'plan.md', 'docs', 'docs/deep', 'docsx/a.md'])).toEqual(['plan.md', 'docs', 'docsx/a.md'])
  })

  it('lists the top-level entries a "delete all" removes', () => {
    expect(topLevelPaths([dir('docs'), file('docs/a.md'), file('plan.md'), dir('node_modules', true)])).toEqual(['docs', 'plan.md', 'node_modules'])
  })

  it('tells whether a path is a removed path or inside one', () => {
    expect(isPathWithin('docs/a.md', 'docs')).toBe(true)
    expect(isPathWithin('docs', 'docs')).toBe(true)
    expect(isPathWithin('docsx/a.md', 'docs')).toBe(false)
  })
})

describe('selection coverage', () => {
  const picked = new Set(['docs', 'src/app/main.ts'])

  it('covers only what lies strictly inside a picked folder', () => {
    expect(isCoveredBySelection('docs/deep/notes.md', picked)).toBe(true)
    expect(isCoveredBySelection('docs', picked)).toBe(false)
    expect(isCoveredBySelection('docsx/a.md', picked)).toBe(false)
  })

  it('finds a pick inside a folder, never the folder itself', () => {
    expect(hasSelectionWithin('src', picked)).toBe(true)
    expect(hasSelectionWithin('src/app/main.ts', picked)).toBe(false)
    expect(hasSelectionWithin('lib', picked)).toBe(false)
  })
})
