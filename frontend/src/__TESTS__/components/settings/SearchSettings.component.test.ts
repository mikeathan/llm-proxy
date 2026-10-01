import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import SearchSettings from '../../../components/settings/SearchSettings.vue'
import { AdminApiService } from '../../../services/admin/adminService'
import type { GlobalConfig } from '../../../types/admin'
import { fieldByLabel } from '../../helpers/fieldByLabel'

const confirm = vi.fn()
vi.mock('../../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm }) }))
vi.mock('../../../services/admin/adminService', () => ({
  AdminApiService: {
    fetchToolSecret: vi.fn(),
    saveToolSecret: vi.fn(),
    deleteToolSecret: vi.fn(),
  },
}))

function makeConfig(overrides: Partial<GlobalConfig> = {}): GlobalConfig {
  return {
    providers: {},
    model_host: '127.0.0.1',
    idle_timeout_seconds: 0,
    guardrails: {
      global: { block_secrets: false, user_blocked_patterns: [] },
      terminal: { enabled: false, allowed_commands: [], timeout_seconds: 0, session_idle_timeout_seconds: 0, max_output_size_chars: 0 },
      search: { enabled: false, max_query_len: 0, blocked_sites: [] },
      communication: { enabled: false, require_review: false, max_messages_per_task: 0 },
      filesystem: { enabled: false, allowed_paths: [], read_only: false, max_file_size_kb: 0 },
      network: { enabled: false, allow_lan_access: false, allow_internet_access: false, max_fetch_size_kb: 0, timeout_seconds: 0 },
    },
    communication: { connectors: {} },
    search: {},
    search_providers: ['tavily', 'brave', 'serpapi'],
    agent_defaults: {
      max_steps: 25,
      context_budget: 8000,
      max_tokens: 2048,
      temperature: 0.1,
      reasoning_budget: 0,
      timeout_minutes: 30,
      tool_call_format: '',
      prefill: false,
      tool_timeout_seconds: 120,
      filesystem_tool_timeout_seconds: 30,
      max_plan_duration_minutes: 15,
      max_plan_steps: 50,
      guardrail_timeout_seconds: 5,
      guardrail_timeout_behavior: 'fail-open',
      guardrail_approval_timeout_seconds: 300,
      loop_strategy: '',
    },
    ...overrides,
  }
}

// VTU's emitted() does not record script-setup emits in this harness, so these
// tests assert the listener callbacks directly — the component's public contract.
function mountSettings(config: GlobalConfig = makeConfig()) {
  const onUpdate = vi.fn()
  const onConfig = vi.fn()
  const wrapper = mount(SearchSettings, {
    props: { editConfig: config },
    attrs: { 'onUpdate:editConfig': onUpdate, onUpdateConfig: onConfig },
  })
  return { wrapper, onUpdate, onConfig }
}

type Wrapper = ReturnType<typeof mountSettings>['wrapper']
const providerSelect = (w: Wrapper) => fieldByLabel(w, /^provider$/i)
const keyInput = (w: Wrapper) => w.find<HTMLInputElement>('input[autocomplete="off"][placeholder*="API key"]')
const keyMask = (w: Wrapper) => w.find('[data-test="key-mask"]')
const button = (w: Wrapper, name: RegExp) =>
  w.findAll('button').find((b) => name.test(b.text()) || name.test(b.attributes('aria-label') ?? ''))!

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(AdminApiService.fetchToolSecret).mockResolvedValue('')
})

describe('SearchSettings', () => {
  it('renders the backend-driven provider list and defaults to the first provider', async () => {
    const { wrapper } = mountSettings(makeConfig({ search_providers: ['alpha', 'beta'] }))
    await flushPromises()

    const options = providerSelect(wrapper).findAll('option').map((o) => o.text())
    expect(options).toEqual(['alpha', 'beta'])
    expect((providerSelect(wrapper).element as HTMLSelectElement).value).toBe('alpha')
  })

  it('falls back to the local provider set when the backend omits search_providers', async () => {
    const { wrapper } = mountSettings(makeConfig({ search_providers: undefined }))
    await flushPromises()

    const options = providerSelect(wrapper).findAll('option').map((o) => o.text())
    expect(options).toEqual(['Tavily', 'Brave Search', 'SerpAPI'])
  })

  it('saves the key for the selected provider and signals the config update', async () => {
    vi.mocked(AdminApiService.saveToolSecret).mockResolvedValue('mock...mask')
    const { wrapper, onConfig } = mountSettings()
    await flushPromises()

    await keyInput(wrapper).setValue('my-tavily-key')
    await button(wrapper, /^save search settings$/i).trigger('click')
    await flushPromises()

    expect(AdminApiService.saveToolSecret).toHaveBeenCalledWith('search', 'tavily', 'my-tavily-key')
    expect(onConfig).toHaveBeenCalledTimes(1)
  })

  it('clears the stored key for the selected provider after confirmation', async () => {
    vi.mocked(AdminApiService.fetchToolSecret).mockResolvedValue('tvly...9f2c')
    vi.mocked(AdminApiService.deleteToolSecret).mockResolvedValue('')
    confirm.mockResolvedValue(true)
    const { wrapper, onConfig } = mountSettings()
    await flushPromises()

    await button(wrapper, /remove stored key/i).trigger('click')
    await flushPromises()

    expect(confirm).toHaveBeenCalled()
    expect(AdminApiService.deleteToolSecret).toHaveBeenCalledWith('search', 'tavily')
    expect(keyMask(wrapper).exists()).toBe(false)
    expect(keyInput(wrapper).exists()).toBe(true)
    expect(onConfig).toHaveBeenCalled()
  })

  it('does not delete the key when the confirmation is dismissed', async () => {
    vi.mocked(AdminApiService.fetchToolSecret).mockResolvedValue('tvly...9f2c')
    confirm.mockResolvedValue(false)
    const { wrapper } = mountSettings()
    await flushPromises()

    await button(wrapper, /remove stored key/i).trigger('click')
    await flushPromises()

    expect(AdminApiService.deleteToolSecret).not.toHaveBeenCalled()
    expect(keyMask(wrapper).text()).toBe('••••••••9f2c')
  })

  it('emits a config update carrying the new provider when the selection changes', async () => {
    const { wrapper, onUpdate } = mountSettings()
    await flushPromises()

    await providerSelect(wrapper).setValue('brave')

    expect(onUpdate).toHaveBeenCalledTimes(1)
    expect((onUpdate.mock.calls[0]?.[0] as GlobalConfig).search?.provider).toBe('brave')
  })

  it('loads the new provider key when the selected provider prop changes', async () => {
    const { wrapper } = mountSettings()
    await flushPromises()
    expect(AdminApiService.fetchToolSecret).toHaveBeenCalledWith('search', 'tavily')

    await wrapper.setProps({ editConfig: makeConfig({ search: { provider: 'brave' } }) })
    await flushPromises()

    expect(AdminApiService.fetchToolSecret).toHaveBeenCalledWith('search', 'brave')
  })

  it('emits the max_results change as a config update', async () => {
    const { wrapper, onUpdate } = mountSettings()
    await flushPromises()

    await fieldByLabel(wrapper, /max results/i).setValue('12')

    expect(onUpdate).toHaveBeenCalledTimes(1)
    expect((onUpdate.mock.calls[0]?.[0] as GlobalConfig).search?.max_results).toBe(12)
  })

  it('shows an error and does not signal a config update when the key save fails', async () => {
    vi.mocked(AdminApiService.saveToolSecret).mockRejectedValue(new Error('boom'))
    const { wrapper, onConfig } = mountSettings()
    await flushPromises()

    await keyInput(wrapper).setValue('my-key')
    await button(wrapper, /^save search settings$/i).trigger('click')
    await flushPromises()

    expect(onConfig).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('Failed to save token')
  })

  it('shows a stored key in the standard masked form, not as editable text', async () => {
    vi.mocked(AdminApiService.fetchToolSecret).mockResolvedValue('tvly...abcd')
    const { wrapper } = mountSettings()
    await flushPromises()

    // Same shape as every other provider key: bullets + last 4.
    expect(keyMask(wrapper).text()).toBe('••••••••abcd')
    // The mask must not be an editable input carrying the raw backend value.
    expect(keyInput(wrapper).exists()).toBe(false)
  })

  it('shows no mask for an unconfigured provider and allows entering a key', async () => {
    vi.mocked(AdminApiService.fetchToolSecret).mockResolvedValue('')
    const { wrapper } = mountSettings()
    await flushPromises()

    expect(keyMask(wrapper).exists()).toBe(false)
    expect(keyInput(wrapper).exists()).toBe(true)
  })

  it('replaces a stored key from a blank field so the mask is never resubmitted', async () => {
    vi.mocked(AdminApiService.fetchToolSecret).mockResolvedValue('tvly...abcd')
    vi.mocked(AdminApiService.saveToolSecret).mockResolvedValue('tvly...wxyz')
    const { wrapper } = mountSettings()
    await flushPromises()

    await button(wrapper, /^replace key$/i).trigger('click')
    // Field starts empty — the mask is not pre-filled.
    expect((keyInput(wrapper).element as HTMLInputElement).value).toBe('')

    await keyInput(wrapper).setValue('tvly-dev-NEWKEYwxyz')
    await button(wrapper, /^save search settings$/i).trigger('click')
    await flushPromises()

    expect(AdminApiService.saveToolSecret).toHaveBeenCalledWith('search', 'tavily', 'tvly-dev-NEWKEYwxyz')
    expect(keyMask(wrapper).text()).toBe('••••••••wxyz')
  })

  it('restores the mask when a replacement is cancelled', async () => {
    vi.mocked(AdminApiService.fetchToolSecret).mockResolvedValue('tvly...abcd')
    const { wrapper } = mountSettings()
    await flushPromises()

    await button(wrapper, /^replace key$/i).trigger('click')
    await keyInput(wrapper).setValue('typed-but-abandoned')
    await button(wrapper, /^cancel$/i).trigger('click')
    await flushPromises()

    expect(keyMask(wrapper).text()).toBe('••••••••abcd')
    expect(AdminApiService.saveToolSecret).not.toHaveBeenCalled()
  })

  it('does not save a replacement that was cancelled', async () => {
    vi.mocked(AdminApiService.fetchToolSecret).mockResolvedValue('tvly...abcd')
    const { wrapper, onUpdate } = mountSettings()
    await flushPromises()

    await button(wrapper, /^replace key$/i).trigger('click')
    await keyInput(wrapper).setValue('typed-but-abandoned')
    await button(wrapper, /^cancel$/i).trigger('click')
    // Something else changed, so there is a save to make — without the key.
    await providerSelect(wrapper).setValue('brave')
    await wrapper.setProps({ editConfig: onUpdate.mock.lastCall![0], configDirty: true })
    await flushPromises()
    await button(wrapper, /^save search settings$/i).trigger('click')
    await flushPromises()

    expect(AdminApiService.saveToolSecret).not.toHaveBeenCalled()
  })

  it('offers Save only while something is unsaved, and reports a typed key to the page', async () => {
    const onDirty = vi.fn()
    const wrapper = mount(SearchSettings, { props: { editConfig: makeConfig() }, attrs: { onDirtyChange: onDirty } })
    await flushPromises()
    expect(button(wrapper, /^save search settings$/i).attributes('disabled')).toBeDefined()

    await keyInput(wrapper).setValue('my-key')
    expect(onDirty).toHaveBeenLastCalledWith(true)
    expect(button(wrapper, /^save search settings$/i).attributes('disabled')).toBeUndefined()

    await button(wrapper, /discard changes/i).trigger('click')
    expect(onDirty).toHaveBeenLastCalledWith(false)
  })
})
