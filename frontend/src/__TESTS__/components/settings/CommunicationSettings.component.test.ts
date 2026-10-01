import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { AdminApiService } from '../../../services/admin/adminService'
import { DEFAULT_CONFIG } from '../../../composables/models/useConfig'
import { fieldByLabel } from '../../helpers/fieldByLabel'
import type { ConnectorConfig, GlobalConfig } from '../../../types/admin'

vi.mock('../../../services/admin/adminService', () => ({
  AdminApiService: { fetchToolSecret: vi.fn(), saveToolSecret: vi.fn(), deleteToolSecret: vi.fn() },
}))
const webhook = { verifyWebhook: vi.fn(), clearWebhookState: vi.fn() }
vi.mock('../../../composables/useWebhook', () => ({ useWebhook: () => webhook }))
const confirm = vi.fn()
vi.mock('../../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm }) }))

import CommunicationSettings from '../../../components/settings/CommunicationSettings.vue'

// happy-dom does not submit a form when its submit button is clicked (browsers
// do), so the tests submit the connector form directly.
const TELEGRAM: ConnectorConfig = { type: 'telegram', enabled: true, settings: { chat_id: '42', workspace_id: 'ws' }, secret_ref: 'tg' }

// The page owns the config: the harness feeds each update back as the new prop,
// like SettingsView's v-model does.
async function mountCommunication(connectors: Record<string, ConnectorConfig> = {}) {
  const onSave = vi.fn()
  const config: GlobalConfig = { ...structuredClone(DEFAULT_CONFIG), communication: { connectors } }
  const w = mount(CommunicationSettings, {
    props: { editConfig: config },
    attrs: {
      'onUpdate:editConfig': (c: GlobalConfig) => w.setProps({ editConfig: c }),
      onUpdateConfig: onSave,
    },
    global: { stubs: { WebhookPanel: true, Icon: true } },
  })
  await flushPromises()
  const connectorsNow = () => (w.props('editConfig') as GlobalConfig).communication?.connectors ?? {}
  const button = (text: RegExp) => w.findAll('button').find((b) => text.test(b.text()) || text.test(b.attributes('aria-label') ?? '') || text.test(b.attributes('title') ?? ''))!
  return { w, onSave, connectorsNow, button }
}

describe('CommunicationSettings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(AdminApiService.fetchToolSecret).mockResolvedValue('')
    vi.mocked(AdminApiService.saveToolSecret).mockResolvedValue('••••1234')
    vi.mocked(AdminApiService.deleteToolSecret).mockResolvedValue('')
    confirm.mockResolvedValue(true)
  })

  it('says when no connector is configured', async () => {
    const { w } = await mountCommunication()
    expect(w.text()).toMatch(/no connectors/i)
  })

  it('loads stored tokens and checks inbound webhooks of existing connectors', async () => {
    await mountCommunication({ tg: TELEGRAM })
    expect(AdminApiService.fetchToolSecret).toHaveBeenCalledWith('connector', 'tg')
    expect(webhook.verifyWebhook).toHaveBeenCalledWith('tg')
  })

  it('adds an enabled connector, then saves its token before the config', async () => {
    const { w, onSave, connectorsNow, button } = await mountCommunication()
    await button(/add connector/i).trigger('click')
    await fieldByLabel(w, /connector name/i).setValue('alerts')
    await fieldByLabel(w, /chat id/i).setValue('42')
    await fieldByLabel(w, /^bot token/i).setValue('123:abc')
    await w.get('form').trigger('submit')
    expect(connectorsNow().alerts).toEqual({ type: 'telegram', enabled: true, settings: { chat_id: '42' }, secret_ref: 'alerts' })

    await button(/save/i).trigger('click')
    await flushPromises()
    expect(AdminApiService.saveToolSecret).toHaveBeenCalledWith('connector', 'alerts', '123:abc')
    expect(onSave).toHaveBeenCalledTimes(1)
  })

  it('does not save the config when a token fails to save', async () => {
    vi.mocked(AdminApiService.saveToolSecret).mockRejectedValue(new Error('vault locked'))
    const { w, onSave, button } = await mountCommunication()
    await button(/add connector/i).trigger('click')
    await fieldByLabel(w, /connector name/i).setValue('alerts')
    await fieldByLabel(w, /^bot token/i).setValue('123:abc')
    await w.get('form').trigger('submit')
    await button(/save/i).trigger('click')
    await flushPromises()
    expect(onSave).not.toHaveBeenCalled()
    expect(w.text()).toContain('vault locked')
  })

  it('switches a connector off', async () => {
    const { w, connectorsNow } = await mountCommunication({ tg: TELEGRAM })
    await w.find('input[type="checkbox"]').setValue(false)
    expect(connectorsNow().tg?.enabled).toBe(false)
  })

  it('removes a connector with its stored token', async () => {
    const { connectorsNow, button } = await mountCommunication({ tg: TELEGRAM })
    await button(/remove/i).trigger('click')
    await flushPromises()
    expect(connectorsNow().tg).toBeUndefined()
    expect(AdminApiService.deleteToolSecret).toHaveBeenCalledWith('connector', 'tg')
    expect(webhook.clearWebhookState).toHaveBeenCalledWith('tg')
  })

  it('refuses to add a connector under a name that exists', async () => {
    const { w, button } = await mountCommunication({ tg: TELEGRAM })
    await button(/add connector/i).trigger('click')
    await fieldByLabel(w, /connector name/i).setValue('tg')
    expect(w.get('[role="alert"]').text()).toMatch(/already exists/)
    expect(button(/^add connector$/i).attributes('disabled')).toBeDefined()
  })

  it('never puts the stored token into the edit form', async () => {
    vi.mocked(AdminApiService.fetchToolSecret).mockResolvedValue('••••9999')
    const { w, button, onSave } = await mountCommunication({ tg: TELEGRAM })
    await button(/^edit tg$/i).trigger('click')
    expect((fieldByLabel(w, /^bot token/i).element as HTMLInputElement).value).toBe('')
    await w.get('form').trigger('submit')
    // Nothing changed, so there is nothing to save.
    expect(button(/^save/i).attributes('disabled')).toBeDefined()
    expect(onSave).not.toHaveBeenCalled()
  })
})
