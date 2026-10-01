import { describe, it, expect, afterEach } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import FileTree from '../../../../components/common/display/FileTree.vue'
import { buildFileTree } from '../../../../utils/workspace/fileTree'
import type { TreeEntry } from '../../../../types/workspace'

const ENTRIES: TreeEntry[] = [
  { path: 'docs', type: 'dir' },
  { path: 'docs/deep', type: 'dir' },
  { path: 'docs/deep/notes.md', type: 'file' },
  { path: 'docs/plan.md', type: 'file' },
  { path: 'node_modules', type: 'dir', collapsed: true },
  { path: 'AGENTS.md', type: 'file' },
]

const mounted: VueWrapper[] = []
function mountTree(props: Record<string, unknown> = {}) {
  const wrapper = mount(FileTree, {
    props: { nodes: buildFileTree(ENTRIES), selectedPath: null, label: 'Files in ws', ...props },
    slots: { actions: '<button class="row-action">x</button>' },
    global: { stubs: { Icon: true } },
    attachTo: document.body,
  })
  mounted.push(wrapper)
  return wrapper
}
const item = (w: VueWrapper, path: string) => w.find(`[role="treeitem"][data-path="${path}"]`)
const visible = (w: VueWrapper) => w.findAll('[role="treeitem"]').map((i) => i.attributes('data-path'))

describe('FileTree', () => {
  afterEach(() => mounted.splice(0).forEach((w) => w.unmount()))

  it('is a labelled tree showing only the top level at first', () => {
    const w = mountTree()
    expect(w.get('[role="tree"]').attributes('aria-label')).toBe('Files in ws')
    expect(visible(w)).toEqual(['docs', 'node_modules', 'AGENTS.md'])
    expect(item(w, 'docs').attributes('aria-expanded')).toBe('false')
    expect(item(w, 'docs').attributes('aria-level')).toBe('1')
  })

  it('reveals children on expand and hides them on collapse', async () => {
    const w = mountTree()
    await item(w, 'docs').trigger('click')
    expect(visible(w)).toEqual(['docs', 'docs/deep', 'docs/plan.md', 'node_modules', 'AGENTS.md'])
    expect(item(w, 'docs/plan.md').attributes('aria-level')).toBe('2')
    await item(w, 'docs').trigger('click')
    expect(visible(w)).toEqual(['docs', 'node_modules', 'AGENTS.md'])
  })

  it('selects a file and reveals the selected file\'s folders', async () => {
    const w = mountTree({ selectedPath: 'docs/deep/notes.md' })
    expect(item(w, 'docs/deep/notes.md').attributes('aria-selected')).toBe('true')
    await item(w, 'AGENTS.md').trigger('click')
    expect(w.emitted('select')).toEqual([['AGENTS.md']])
  })

  it('shows a heavy directory without expanding it', async () => {
    const w = mountTree()
    const heavy = item(w, 'node_modules')
    expect(heavy.attributes('aria-expanded')).toBeUndefined()
    expect(heavy.text()).toContain('not listed')
    await heavy.trigger('click')
    expect(visible(w)).toEqual(['docs', 'node_modules', 'AGENTS.md'])
  })

  it('moves with the arrow keys, expands, returns to the parent and opens files', async () => {
    const w = mountTree()
    expect(item(w, 'docs').attributes('tabindex')).toBe('0')
    await item(w, 'docs').trigger('keydown', { key: 'ArrowRight' })
    expect(item(w, 'docs').attributes('aria-expanded')).toBe('true')
    await item(w, 'docs').trigger('keydown', { key: 'ArrowRight' })
    expect(document.activeElement?.getAttribute('data-path')).toBe('docs/deep')
    await item(w, 'docs/deep').trigger('keydown', { key: 'ArrowDown' })
    expect(document.activeElement?.getAttribute('data-path')).toBe('docs/plan.md')
    expect(item(w, 'docs/plan.md').attributes('tabindex')).toBe('0')
    await item(w, 'docs/plan.md').trigger('keydown', { key: 'Enter' })
    expect(w.emitted('select')).toEqual([['docs/plan.md']])
    await item(w, 'docs/plan.md').trigger('keydown', { key: 'ArrowLeft' })
    expect(document.activeElement?.getAttribute('data-path')).toBe('docs')
    await item(w, 'docs').trigger('keydown', { key: 'End' })
    expect(document.activeElement?.getAttribute('data-path')).toBe('AGENTS.md')
  })

  it('ignores keys pressed on a row action', async () => {
    const w = mountTree()
    await w.get('[data-path="AGENTS.md"] .row-action').trigger('keydown', { key: 'Enter' })
    expect(w.emitted('select')).toBeUndefined()
  })

  it('says when the listing was truncated, and when there are no files', () => {
    expect(mountTree({ truncated: true }).get('[role="status"]').text()).toContain('deeper files are not shown')
    expect(mountTree({ nodes: [] }).text()).toContain('No files yet')
  })
})
