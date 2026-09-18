import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import SearchSettings from '../../../components/settings/SearchSettings.vue'
import { AdminApiService } from '../../../services/admin/adminService'
import type { GlobalConfig } from '../../../types/admin'

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

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(AdminApiService.fetchToolSecret).mockResolvedValue('')
})

describe('SearchSettings', () => {
  it('renders the backend-driven provider list and defaults to the first provider', async () => {
    const { wrapper } = mountSettings(makeConfig({ search_providers: ['alpha', 'beta'] }))
    await flushPromises()

    const options = wrapper.findAll('#search-provider option').map((o) => o.text())
    expect(options).toEqual(['alpha', 'beta'])
    expect((wrapper.get('#search-provider').element as HTMLSelectElement).value).toBe('alpha')
  })

  it('falls back to the local provider set when the backend omits search_providers', async () => {
    const { wrapper } = mountSettings(makeConfig({ search_providers: undefined }))
    await flushPromises()

    const options = wrapper.findAll('#search-provider option').map((o) => o.text())
    expect(options).toEqual(['Tavily', 'Brave Search', 'SerpAPI'])
  })

  it('saves the key for the selected provider and signals the config update', async () => {
    vi.mocked(AdminApiService.saveToolSecret).mockResolvedValue('mock...mask')
    const { wrapper, onConfig } = mountSettings()
    await flushPromises()

    await wrapper.get('#search-key').setValue('my-tavily-key')
    await wrapper.get('.save-search').trigger('click')
    await flushPromises()

    expect(AdminApiService.saveToolSecret).toHaveBeenCalledWith('search', 'tavily', 'my-tavily-key')
    expect(onConfig).toHaveBeenCalledTimes(1)
  })

  it('clears the stored key for the selected provider after confirmation', async () => {
    vi.mocked(AdminApiService.fetchToolSecret).mockResolvedValue('tvly...9f2c')
    vi.mocked(AdminApiService.deleteToolSecret).mockResolvedValue('')
    const confirmSpy = vi.fn().mockReturnValue(true)
    window.confirm = confirmSpy
    const { wrapper, onConfig } = mountSettings()
    await flushPromises()

    await wrapper.get('.clear-search-key').trigger('click')
    await flushPromises()

    expect(confirmSpy).toHaveBeenCalled()
    expect(AdminApiService.deleteToolSecret).toHaveBeenCalledWith('search', 'tavily')
    expect(wrapper.find('.key-mask').exists()).toBe(false)
    expect(wrapper.find('#search-key').exists()).toBe(true)
    expect(onConfig).toHaveBeenCalled()
  })

  it('does not delete the key when the confirmation is dismissed', async () => {
    vi.mocked(AdminApiService.fetchToolSecret).mockResolvedValue('tvly...9f2c')
    window.confirm = vi.fn().mockReturnValue(false)
    const { wrapper } = mountSettings()
    await flushPromises()

    await wrapper.get('.clear-search-key').trigger('click')
    await flushPromises()

    expect(AdminApiService.deleteToolSecret).not.toHaveBeenCalled()
    expect(wrapper.get('.key-mask').text()).toBe('••••••••9f2c')
  })

  it('emits a config update carrying the new provider when the selection changes', async () => {
    const { wrapper, onUpdate } = mountSettings()
    await flushPromises()

    await wrapper.get('#search-provider').setValue('brave')

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

    await wrapper.get('#search-max').setValue('12')

    expect(onUpdate).toHaveBeenCalledTimes(1)
    expect((onUpdate.mock.calls[0]?.[0] as GlobalConfig).search?.max_results).toBe(12)
  })

  it('shows an error and does not signal a config update when the key save fails', async () => {
    vi.mocked(AdminApiService.saveToolSecret).mockRejectedValue(new Error('boom'))
    const { wrapper, onConfig } = mountSettings()
    await flushPromises()

    await wrapper.get('#search-key').setValue('my-key')
    await wrapper.get('.save-search').trigger('click')
    await flushPromises()

    expect(onConfig).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('Failed to save token')
  })

  it('shows a stored key in the standard masked form, not as editable text', async () => {
    vi.mocked(AdminApiService.fetchToolSecret).mockResolvedValue('tvly...abcd')
    const { wrapper } = mountSettings()
    await flushPromises()

    // Same shape as every other provider key: bullets + last 4.
    expect(wrapper.get('.key-mask').text()).toBe('••••••••abcd')
    // The mask must not be an editable input carrying the raw backend value.
    expect(wrapper.find('#search-key').exists()).toBe(false)
  })

  it('shows no mask for an unconfigured provider and allows entering a key', async () => {
    vi.mocked(AdminApiService.fetchToolSecret).mockResolvedValue('')
    const { wrapper } = mountSettings()
    await flushPromises()

    expect(wrapper.find('.key-mask').exists()).toBe(false)
    expect(wrapper.find('#search-key').exists()).toBe(true)
  })

  it('replaces a stored key from a blank field so the mask is never resubmitted', async () => {
    vi.mocked(AdminApiService.fetchToolSecret).mockResolvedValue('tvly...abcd')
    vi.mocked(AdminApiService.saveToolSecret).mockResolvedValue('tvly...wxyz')
    const { wrapper } = mountSettings()
    await flushPromises()

    await wrapper.get('.replace-search-key').trigger('click')
    // Field starts empty — the mask is not pre-filled.
    expect((wrapper.get('#search-key').element as HTMLInputElement).value).toBe('')

    await wrapper.get('#search-key').setValue('tvly-dev-NEWKEYwxyz')
    await wrapper.get('.save-search').trigger('click')
    await flushPromises()

    expect(AdminApiService.saveToolSecret).toHaveBeenCalledWith('search', 'tavily', 'tvly-dev-NEWKEYwxyz')
    expect(wrapper.get('.key-mask').text()).toBe('••••••••wxyz')
  })

  it('restores the mask when a replacement is cancelled', async () => {
    vi.mocked(AdminApiService.fetchToolSecret).mockResolvedValue('tvly...abcd')
    const { wrapper } = mountSettings()
    await flushPromises()

    await wrapper.get('.replace-search-key').trigger('click')
    await wrapper.get('#search-key').setValue('typed-but-abandoned')
    await wrapper.get('.cancel-search-key').trigger('click')
    await flushPromises()

    expect(wrapper.get('.key-mask').text()).toBe('••••••••abcd')
    expect(AdminApiService.saveToolSecret).not.toHaveBeenCalled()
  })
})
