import { describe, it, expect, afterEach } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import type { Plugin } from 'vue'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '../../../../router'
import { useConfirm } from '../../../../composables/ui/useConfirm'
import WorkspaceList from '../../../../components/AgentIde/workspace/WorkspaceList.vue'
import WorkspaceHeader from '../../../../components/AgentIde/workspace/WorkspaceHeader.vue'
import WorkspaceFiles from '../../../../components/AgentIde/workspace/WorkspaceFiles.vue'
import FileEditor from '../../../../components/AgentIde/workspace/FileEditor.vue'
import type { WorkspaceTree } from '../../../../types/workspace'

const mounted: VueWrapper[] = []
async function withRouter<T extends VueWrapper>(make: (plugins: Plugin[]) => T, path = '/workspaces'): Promise<T> {
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  const w = make([router])
  mounted.push(w)
  await flushPromises()
  return w
}
const stubs = { Icon: true, BrandMark: true }
const button = (w: VueWrapper, text: string) => w.findAll('button').find((b) => b.text().trim() === text || b.attributes('aria-label') === text)!

afterEach(() => {
  mounted.splice(0).forEach((w) => w.unmount())
  document.body.innerHTML = ''
})

describe('WorkspaceList', () => {
  const mountList = (props: Record<string, unknown> = {}) =>
    withRouter((plugins) => mount(WorkspaceList, {
      props: { workspaces: [{ id: 'demo' }, { id: 'lab' }], externalAccess: { lab: true }, loading: false, ...props },
      global: { plugins, stubs },
      attachTo: document.body,
    }))

  it('links every workspace and marks external access', async () => {
    const w = await mountList()
    expect(w.findAll('tbody a').map((a) => [a.text(), a.attributes('href')])).toEqual([['demo', '/workspaces/demo'], ['lab', '/workspaces/lab']])
    expect(w.findAll('tbody tr')[1]!.text()).toContain('External access')
  })

  it('creates a workspace with a valid name and refuses an invalid one', async () => {
    const w = await mountList()
    await w.get('input[name="workspace-name"]').setValue('bad name!')
    await w.get('form').trigger('submit')
    expect(w.emitted('create')).toBeUndefined()
    expect(w.get('[role="alert"]').text()).toContain('letters, digits, dashes and underscores')
    await w.get('input[name="workspace-name"]').setValue('research')
    await w.get('form').trigger('submit')
    expect(w.emitted('create')).toEqual([['research']])
  })

  it('deletes a workspace only after confirming', async () => {
    const w = await mountList()
    await button(w, 'Delete workspace lab').trigger('click')
    await flushPromises()
    expect(w.emitted('delete')).toBeUndefined()
    useConfirm().handleConfirm()
    await flushPromises()
    expect(w.emitted('delete')).toEqual([['lab']])
  })

  it('invites creating the first workspace when there are none', async () => {
    expect((await mountList({ workspaces: [] })).text()).toContain('No workspaces yet')
  })
})

describe('WorkspaceHeader', () => {
  const mountHeader = (props: Record<string, unknown> = {}) =>
    withRouter((plugins) => mount(WorkspaceHeader, {
      props: { ws: 'demo', workspaces: [{ id: 'demo' }, { id: 'lab' }], section: 'memory', chatRunning: false, externalAccess: false, ...props },
      global: { plugins, stubs },
    }), '/workspaces/demo/memory')

  it('links each section of the workspace, marking the current one', async () => {
    const w = await mountHeader()
    const links = w.findAll('nav a')
    expect(links.map((a) => [a.text(), a.attributes('href')])).toEqual([
      ['Files', '/workspaces/demo/files'],
      ['Assistant', '/workspaces/demo/assistant'],
      ['Memory', '/workspaces/demo/memory'],
      ['Playbooks', '/workspaces/demo/playbooks'],
      ['Settings', '/workspaces/demo/settings'],
    ])
    expect(links.filter((a) => a.attributes('aria-current') === 'page').map((a) => a.text())).toEqual(['Memory'])
  })

  it('switches workspace and flags a running chat', async () => {
    const w = await mountHeader({ chatRunning: true })
    await w.get('select').setValue('lab')
    expect(w.emitted('switch')).toEqual([['lab']])
    expect(w.get('nav').text()).toContain('running')
  })
})

describe('WorkspaceFiles', () => {
  const TREE: WorkspaceTree = {
    entries: [
      { path: 'docs', type: 'dir' },
      { path: 'docs/deep', type: 'dir' },
      { path: 'docs/deep/notes.md', type: 'file' },
      { path: 'plan.md', type: 'file' },
    ],
    truncated: false,
  }
  const mountFiles = (props: Record<string, unknown> = {}) =>
    withRouter((plugins) => mount(WorkspaceFiles, {
      props: { workspace: 'demo', tree: TREE, selectedPath: null, loading: false, ...props },
      global: { plugins, stubs },
      attachTo: document.body,
    }))

  it('opens a file from the tree', async () => {
    const w = await mountFiles()
    await w.get('[data-path="plan.md"]').trigger('click')
    expect(w.emitted('open-file')).toEqual([['plan.md']])
  })

  it('filters to matching files, revealing the folders they are in', async () => {
    const w = await mountFiles()
    await w.get('input[type="search"]').setValue('notes')
    expect(w.findAll('[role="treeitem"]').map((i) => i.attributes('data-path'))).toEqual(['docs', 'docs/deep', 'docs/deep/notes.md'])
  })

  it('creates a nested file and refuses an unsafe path', async () => {
    const w = await mountFiles()
    await w.get('input[name="new-file"]').setValue('../escape.md')
    await w.get('form').trigger('submit')
    expect(w.emitted('create-file')).toBeUndefined()
    expect(w.get('[role="alert"]').text()).toContain('inside the workspace')
    await w.get('input[name="new-file"]').setValue('drafts/idea.md')
    await w.get('form').trigger('submit')
    expect(w.emitted('create-file')).toEqual([['drafts/idea.md']])
  })

  it('deletes a file only after confirming', async () => {
    const w = await mountFiles()
    await button(w, 'Delete plan.md').trigger('click')
    await flushPromises()
    expect(w.emitted('delete-file')).toBeUndefined()
    useConfirm().handleConfirm()
    await flushPromises()
    expect(w.emitted('delete-paths')).toEqual([[['plan.md']]])
  })

  it('deletes a folder with everything in it only after confirming', async () => {
    const w = await mountFiles()
    await button(w, 'Delete folder docs').trigger('click')
    await flushPromises()
    expect(useConfirm().options.value.message).toContain('everything inside it')
    useConfirm().handleConfirm()
    await flushPromises()
    expect(w.emitted('delete-paths')).toEqual([[['docs']]])
  })

  it('selects files and folders, then deletes the selection together', async () => {
    const w = await mountFiles()
    expect(w.find('input[type="checkbox"]').exists()).toBe(false)
    await button(w, 'Select').trigger('click')
    await w.get('input[aria-label="Select docs"]').setValue(true)
    await w.get('input[aria-label="Select plan.md"]').setValue(true)
    expect(w.text()).toContain('2 selected')
    await button(w, 'Delete selected').trigger('click')
    await flushPromises()
    useConfirm().handleConfirm()
    await flushPromises()
    expect(w.emitted('delete-paths')).toEqual([[['docs', 'plan.md']]])
    expect(w.find('input[type="checkbox"]').exists()).toBe(false)
  })

  it('selecting a folder shows everything inside it as selected and locked', async () => {
    const w = await mountFiles()
    await w.get('input[type="search"]').setValue('notes') // expands docs/deep so its rows render
    await button(w, 'Select').trigger('click')
    const box = (path: string) => w.get<HTMLInputElement>(`input[aria-label="Select ${path}"]`)
    await box('docs').setValue(true)
    expect(box('docs/deep').element.checked).toBe(true)
    expect(box('docs/deep/notes.md').element.checked).toBe(true)
    expect(box('docs/deep').element.disabled).toBe(true)
    expect(box('docs').element.disabled).toBe(false)
    expect(w.text()).toContain('1 selected (+2 inside)')
    await box('docs').setValue(false)
    expect(box('docs/deep/notes.md').element.checked).toBe(false)
    expect(box('docs/deep/notes.md').element.disabled).toBe(false)
  })

  it('shows a folder as partly selected when only some of its contents are', async () => {
    const w = await mountFiles()
    await w.get('input[type="search"]').setValue('notes')
    await button(w, 'Select').trigger('click')
    await w.get('input[aria-label="Select docs/deep/notes.md"]').setValue(true)
    const docs = w.get<HTMLInputElement>('input[aria-label="Select docs"]').element
    expect(docs.checked).toBe(false)
    expect(docs.indeterminate).toBe(true)
  })

  it('clicking a checkbox selects the row without opening the file', async () => {
    const w = await mountFiles()
    await button(w, 'Select').trigger('click')
    await w.get('input[aria-label="Select plan.md"]').trigger('click')
    expect(w.emitted('open-file')).toBeUndefined()
  })

  it('select all ticks every top-level entry, and unticks them again', async () => {
    const w = await mountFiles()
    await button(w, 'Select').trigger('click')
    const all = () => w.get<HTMLInputElement>('input[aria-label="Select all"]')
    await all().setValue(true)
    expect(w.text()).toContain('2 selected (+2 inside)')
    expect(all().element.checked).toBe(true)
    await all().setValue(false)
    expect(w.text()).toContain('0 selected')
  })

  it('select all is partly ticked when only some entries are', async () => {
    const w = await mountFiles()
    await button(w, 'Select').trigger('click')
    await w.get('input[aria-label="Select plan.md"]').setValue(true)
    const all = w.get<HTMLInputElement>('input[aria-label="Select all"]').element
    expect(all.checked).toBe(false)
    expect(all.indeterminate).toBe(true)
  })

  it('select all reaches entries a filter hides, then deletes them after confirming', async () => {
    const w = await mountFiles()
    await w.get('input[type="search"]').setValue('notes')
    await button(w, 'Select').trigger('click')
    await w.get('input[aria-label="Select all"]').setValue(true)
    await button(w, 'Delete selected').trigger('click')
    await flushPromises()
    useConfirm().handleConfirm()
    await flushPromises()
    expect(w.emitted('delete-paths')).toEqual([[['docs', 'plan.md']]])
  })

  it('says when the workspace has no files', async () => {
    expect((await mountFiles({ tree: { entries: [], truncated: false } })).text()).toContain('No files yet')
  })

  it('shows at most 200 filter matches, and says how many it left out', async () => {
    const many: WorkspaceTree = { entries: Array.from({ length: 250 }, (_, i) => ({ path: `n-${i}.md`, type: 'file' as const })), truncated: false }
    const w = await mountFiles({ tree: many })
    await w.get('input[type="search"]').setValue('n-')
    expect(w.findAll('[role="treeitem"]')).toHaveLength(200)
    expect(w.get('[role="status"]').text()).toContain('Showing 200 of 250 matches')
  })
})

describe('FileEditor', () => {
  it('is a labelled editor that reports edits, and shows loading', async () => {
    const w = mount(FileEditor, { props: { filename: 'plan.md', content: '# Plan', loading: false } })
    const area = w.get('textarea')
    expect(area.attributes('aria-label')).toBe('Contents of plan.md')
    await area.setValue('# Plan v2')
    expect(w.emitted('update:content')).toEqual([['# Plan v2']])
    expect(mount(FileEditor, { props: { filename: 'plan.md', content: '', loading: true } }).find('[role="status"]').exists()).toBe(true)
  })

  it('saves with Ctrl/Cmd+S', async () => {
    const w = mount(FileEditor, { props: { filename: 'plan.md', content: '', loading: false } })
    await w.get('textarea').trigger('keydown', { key: 's', ctrlKey: true })
    await w.get('textarea').trigger('keydown', { key: 's', metaKey: true })
    expect(w.emitted('save')).toHaveLength(2)
  })
})
