import { describe, it, expect, vi, beforeEach } from 'vitest'
import { nextTick, ref } from 'vue'
import type { AdminState } from '../../../types/admin'
import type { Automation } from '../../../types/dispatcher'

const adminState = ref<AdminState | null>(null)
vi.mock('../../../composables/models/useModels', () => ({ useModels: () => ({ state: adminState }) }))

import { useAutomationForm } from '../../../composables/automation/useAutomationForm'

const MODELS = [
  { name: 'qwen', provider: 'local', endpoint: '', active: false, ready: false },
  { name: 'gpt-5', provider: 'openai', endpoint: '', active: false, ready: false, provider_config: { api_key_name: 'personal' } },
  { name: 'gpt-mini', provider: 'openai', endpoint: '', active: false, ready: false, provider_config: { api_key_name: 'personal' } },
]
const STATE = {
  models: MODELS,
  config: { providers: { openai: { api_keys: [{ id: 'k1', name: 'personal' }] } } },
} as unknown as AdminState

const AUTO = {
  id: 'ws/nightly', workspace: 'ws', name: 'nightly', task_file: 'jobs/nightly.md', strategy: 'persistent',
  trigger: 'cron', trigger_value: '0 7 * * *', model: 'gpt-5', loop_strategy: 'react', network_grant: 'lan', memory_mode: 'hot',
} as Automation

// Characterisation (plan D22) — and the tests the absorbed
// automation-edit-form-reactivity plan asked for (populate on edit, reset on
// null, derived provider key, late model load).
describe('useAutomationForm', () => {
  beforeEach(() => {
    adminState.value = STATE
  })

  it('populates every field from the automation being edited, and asks for its workspace files', () => {
    const fetchFiles = vi.fn()
    const f = useAutomationForm(ref(AUTO), fetchFiles)
    expect(f.selectedWorkspace.value).toBe('ws')
    expect(f.form.value).toEqual({
      name: 'nightly', triggerType: 'cron', triggerValue: '0 7 * * *', taskFile: 'jobs/nightly.md',
      strategy: 'persistent', model: 'gpt-5', loopStrategy: 'react', networkGrant: 'lan', memoryMode: 'hot',
      notifyConnector: '', notifyDedup: false, notifyDedupDays: '', notifySendEmpty: false, skipIfBusy: false,
    })
  })

  it('repopulates when a different automation is edited, and resets when editing stops', async () => {
    const edit = ref<Automation | null>(null)
    const f = useAutomationForm(edit, vi.fn())
    expect(f.form.value.name).toBe('')
    edit.value = AUTO
    await nextTick()
    expect(f.form.value.name).toBe('nightly')
    edit.value = null
    await nextTick()
    expect(f.form.value.name).toBe('')
    expect(f.selectedWorkspace.value).toBe('')
  })

  // The owner polls the automation list, so the same automation arrives as a
  // new object every few seconds; that must not wipe the user's edits.
  it('keeps the edits when the same automation is refreshed', async () => {
    const edit = ref<Automation | null>(AUTO)
    const f = useAutomationForm(edit, vi.fn())
    f.form.value.triggerValue = '0 9 * * *'
    edit.value = { ...AUTO }
    await nextTick()
    expect(f.form.value.triggerValue).toBe('0 9 * * *')
  })

  it('derives the connection from the model, and picks a model for a chosen connection', () => {
    const f = useAutomationForm(ref(AUTO), vi.fn())
    expect(f.selectedProviderKey.value).toBe('openai/personal')
    expect(f.filteredModels.value.map((m) => m.name)).toEqual(['gpt-5', 'gpt-mini'])
    f.selectedProviderKey.value = 'local'
    expect(f.form.value.model).toBe('qwen')
    expect(f.cloudProvidersWithKeys.value).toEqual([{ providerName: 'openai', keys: [{ name: 'personal', id: 'k1', keyVal: 'personal' }] }])
  })

  it('fills the connection once models load late', async () => {
    adminState.value = null
    const f = useAutomationForm(ref(AUTO), vi.fn())
    expect(f.selectedProviderKey.value).toBe('')
    adminState.value = STATE
    await nextTick()
    expect(f.selectedProviderKey.value).toBe('openai/personal')
  })

  it('clears the task file when the workspace changes on a new automation', async () => {
    const fetchFiles = vi.fn()
    const f = useAutomationForm(ref(null), fetchFiles)
    f.form.value.taskFile = 'a.md'
    f.selectedWorkspace.value = 'lab'
    await nextTick()
    expect(fetchFiles).toHaveBeenCalledWith('lab')
    expect(f.form.value.taskFile).toBe('')
  })

  it('submits only with a workspace and a name', () => {
    const f = useAutomationForm(ref(null), vi.fn())
    expect(f.handleSubmit()).toBeNull()
    f.selectedWorkspace.value = 'ws'
    f.form.value.name = 'x'
    expect(f.handleSubmit()).toMatchObject({ name: 'x' })
  })
  it('populates delivery and busy behaviour from the automation', () => {
    const f = useAutomationForm(ref({ ...AUTO, notify: { connector: 'my-tg', dedup: true, dedup_days: 14, send_empty: true }, skip_if_busy: true }), vi.fn())
    expect(f.form.value).toMatchObject({ notifyConnector: 'my-tg', notifyDedup: true, notifyDedupDays: '14', notifySendEmpty: true, skipIfBusy: true })
  })

  it('lists configured connectors and keeps the form\'s own connector selectable', () => {
    adminState.value = { ...STATE, config: { ...STATE.config, communication: { connectors: { 'my-tg': { type: 'telegram', enabled: true, settings: {} } } } } } as unknown as AdminState
    const f = useAutomationForm(ref({ ...AUTO, notify: { connector: 'removed' } }), vi.fn())
    expect(f.noConnectors.value).toBe(false)
    expect(f.connectorOptions.value).toEqual([
      { value: 'my-tg', label: 'my-tg' },
      { value: 'removed', label: 'removed (not configured)' },
    ])
  })
  it('reports "no connectors" only once the admin state has loaded', () => {
    adminState.value = null
    const f = useAutomationForm(ref(AUTO), vi.fn())
    expect(f.noConnectors.value).toBe(false)
    adminState.value = STATE
    expect(f.noConnectors.value).toBe(true)
  })
})
