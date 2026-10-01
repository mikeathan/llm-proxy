import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import type { Component } from 'vue'
import DataTable from '../../../../components/common/display/DataTable.vue'
import type { DataTableColumn } from '../../../../types/ui'

interface Run { id: string; name: string; tokens: number }
const COLUMNS: DataTableColumn<Run>[] = [
  { key: 'name', label: 'Automation', value: (r) => r.name },
  { key: 'tokens', label: 'Tokens', numeric: true, value: (r) => r.tokens },
]
const ROWS: Run[] = [{ id: 'a', name: 'nightly', tokens: 1200 }, { id: 'b', name: 'hourly', tokens: 30 }]

function mountTable(props: Record<string, unknown> = {}, slots: Record<string, string> = {}) {
  // mount() cannot infer a generic component's type parameter; props are checked by the assertions.
  return mount(DataTable as Component, {
    props: { columns: COLUMNS, rows: ROWS, rowKey: (r: Run) => r.id, caption: 'Recent runs', ...props },
    slots,
    global: { stubs: { BrandMark: true } },
  })
}

describe('DataTable', () => {
  it('renders a captioned table with column headers and cells', () => {
    const w = mountTable()
    expect(w.get('caption').text()).toBe('Recent runs')
    expect(w.findAll('th').map((th) => [th.text(), th.attributes('scope')])).toEqual([['Automation', 'col'], ['Tokens', 'col']])
    expect(w.findAll('tbody tr')).toHaveLength(2)
    const cells = w.findAll('tbody tr')[0]!.findAll('td')
    expect(cells.map((c) => c.text())).toEqual(['nightly', '1200'])
    expect(cells.map((c) => c.attributes('data-label'))).toEqual(['Automation', 'Tokens'])
    expect(cells[1]!.classes()).toContain('tabular-nums')
  })

  it('lets a cell slot override the value', () => {
    const w = mountTable({}, { 'cell-name': '<template #cell-name="{ row }"><b>{{ row.name.toUpperCase() }}</b></template>' })
    expect(w.find('tbody b').text()).toBe('NIGHTLY')
  })

  it('shows loading, error and empty states instead of rows', () => {
    expect(mountTable({ loading: true }).get('[role="status"]').text()).toContain('Loading recent runs')
    const failed = mountTable({ error: 'The server did not respond.' })
    expect(failed.get('[role="alert"]').text()).toContain('The server did not respond.')
    expect(failed.find('table').exists()).toBe(false)
    expect(mountTable({ rows: [], emptyTitle: 'No runs yet' }).text()).toContain('No runs yet')
  })

  it('makes rows keyboard-activatable when asked', async () => {
    const w = mountTable({ activatable: true })
    const row = w.findAll('tbody tr')[1]!
    expect(row.attributes('tabindex')).toBe('0')
    await row.trigger('keydown', { key: 'Enter' })
    await w.findAll('tbody tr')[0]!.trigger('click')
    expect(w.emitted('activate')).toEqual([[ROWS[1]], [ROWS[0]]])
    expect(mountTable().findAll('tbody tr')[0]!.attributes('tabindex')).toBeUndefined()
  })

  it('leaves Enter on a control inside a row to that control', async () => {
    const w = mountTable({ activatable: true }, { 'cell-name': '<template #cell-name><button class="inner">copy</button></template>' })
    await w.findAll('tbody .inner')[0]!.trigger('keydown', { key: 'Enter' })
    expect(w.emitted('activate')).toBeUndefined()
  })
})
