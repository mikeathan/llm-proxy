import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { AdminApiService } from '../../../services/admin/adminService'
import { DEFAULT_CONFIG } from '../../../composables/models/useConfig'
import { fieldByLabel } from '../../helpers/fieldByLabel'
import type { ConnectorConfig, GlobalConfig } from '../../../types/admin'

vi.mock('../../../services/admin/adminService', () => ({
  AdminApiService: { fetchToolSecret: vi.fn(), saveToolSecret: vi.fn(), deleteToolSecret: vi.fn(), deleteConnectorWebhook: vi.fn() },
}))
const webhook = { verifyWebhook: vi.fn(), clearWebhookState: vi.fn() }
vi.mock('../../../composables/useWebhook', () => ({ useWebhook: () => webhook }))
const confirm = vi.fn()
vi.mock('../../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm }) }))

import CommunicationSettings from '../../../components/settings/CommunicationSettings.vue'

enableAutoUnmount(afterEach)
afterEach(() => { document.body.innerHTML = '' })

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
    attachTo: document.body,
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
    vi.mocked(AdminApiService.deleteConnectorWebhook).mockResolvedValue({ status: 'deleted' })
    confirm.mockResolvedValue(true)
  })

  it('says when no connector is configured', async () => {
    const { w } = await mountCommunication()
    expect(w.text()).toMatch(/no connectors/i)
  })

  it('opens sourced Telegram help without losing the draft, and restores focus when closed', async () => {
    const { w, button } = await mountCommunication()
    await button(/add connector/i).trigger('click')
    await fieldByLabel(w, /connector name/i).setValue('my-draft')
    const helpButton = button(/Telegram setup help/i)
    ;(helpButton.element as HTMLElement).focus()
    await helpButton.trigger('click')
    await flushPromises()
    const dialog = w.get('[role="dialog"]')
    expect(dialog.text()).toContain('message.chat.id')
    expect(dialog.findAll('figure')).toHaveLength(2)
    expect(dialog.findAll('a').every((link) => link.attributes('href')?.startsWith('https://core.telegram.org/'))).toBe(true)
    const close = dialog.get('button')
    expect(document.activeElement).toBe(close.element)
    await close.trigger('keydown', { key: 'Tab', shiftKey: true })
    const links = dialog.findAll('a')
    expect(document.activeElement).toBe(links[links.length - 1]!.element)
    await dialog.trigger('keydown', { key: 'Escape' })
    expect(w.find('[role="dialog"]').exists()).toBe(false)
    expect(document.activeElement).toBe(helpButton.element)
    expect((fieldByLabel(w, /connector name/i).element as HTMLInputElement).value).toBe('my-draft')
    expect(AdminApiService.saveToolSecret).not.toHaveBeenCalled()
    w.unmount()
  })

  it('associates setup guidance with the fields and offers instructions for finding Telegram values', async () => {
    const { w, button } = await mountCommunication()
    await button(/add connector/i).trigger('click')
    for (const [label, guidance] of [
      [/connector name/i, /label you choose/i],
      [/chat id/i, /separate from the bot token/i],
      [/^bot token/i, /including the colon/i],
      [/workspace for inbound/i, /workspace ID/i],
      [/webhook secret token/i, /secret you choose/i],
    ] as const) {
      const control = fieldByLabel(w, label)
      const hint = w.get(`[id="${control.attributes('aria-describedby')}"]`)
      expect(hint.text()).toMatch(guidance)
    }
    const help = w.get('details')
    expect(help.get('summary').text()).toMatch(/find.*Telegram.*values/i)
    expect(help.text()).toContain('message.chat.id')
    expect(help.text()).toMatch(/getUpdates.*webhook/is)
    expect(w.text()).toContain('Save communication settings')
    expect(w.text()).toMatch(/Inbound webhook.*Register/is)
    const token = fieldByLabel(w, /^bot token/i)
    expect(token.attributes('type')).toBe('password')
    expect((token.element as HTMLInputElement).value).toBe('')
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
    expect(AdminApiService.deleteConnectorWebhook).not.toHaveBeenCalled()
  })

  it('unregisters the saved webhook before deleting the bot token', async () => {
    const cfg = { ...TELEGRAM, webhook_url: 'https://example.com/api/v1/webhooks/tg' }
    const { connectorsNow, button } = await mountCommunication({ tg: cfg })
    await button(/remove/i).trigger('click')
    await flushPromises()
    expect(AdminApiService.deleteConnectorWebhook).toHaveBeenCalledWith('tg')
    expect(vi.mocked(AdminApiService.deleteConnectorWebhook).mock.invocationCallOrder[0]).toBeLessThan(vi.mocked(AdminApiService.deleteToolSecret).mock.invocationCallOrder[0]!)
    expect(connectorsNow().tg).toBeUndefined()
  })

  it('keeps the connector and token when unregistering its webhook fails', async () => {
    const cfg = { ...TELEGRAM, webhook_url: 'https://example.com/api/v1/webhooks/tg' }
    vi.mocked(AdminApiService.deleteConnectorWebhook).mockRejectedValue(new Error('Telegram unavailable'))
    const { w, connectorsNow, button } = await mountCommunication({ tg: cfg })
    await button(/remove/i).trigger('click')
    await flushPromises()
    expect(connectorsNow().tg).toEqual(cfg)
    expect(AdminApiService.deleteToolSecret).not.toHaveBeenCalled()
    expect(w.text()).toContain('Telegram unavailable')
  })

  it('keeps the connector without its unregistered URL if deleting its token fails', async () => {
    const cfg = { ...TELEGRAM, webhook_url: 'https://example.com/api/v1/webhooks/tg' }
    vi.mocked(AdminApiService.deleteToolSecret).mockRejectedValue(new Error('vault unavailable'))
    const { w, connectorsNow, button } = await mountCommunication({ tg: cfg })
    await button(/remove/i).trigger('click')
    await flushPromises()
    expect(connectorsNow().tg).toBeDefined()
    expect(connectorsNow().tg?.webhook_url).toBeUndefined()
    expect(w.text()).toContain('vault unavailable')
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
    expect(fieldByLabel(w, /^bot token/i).attributes('placeholder')).toBe('••••9999')
    await w.get('form').trigger('submit')
    // Nothing changed, so there is nothing to save.
    expect(button(/^save/i).attributes('disabled')).toBeDefined()
    expect(onSave).not.toHaveBeenCalled()
  })

  it('masks and preserves a saved webhook secret when editing another field', async () => {
    const cfg = { ...TELEGRAM, settings: { ...TELEGRAM.settings, webhook_token: 'test-webhook-secret' } }
    const { w, button, connectorsNow } = await mountCommunication({ tg: cfg })
    await button(/^edit tg$/i).trigger('click')
    const secret = fieldByLabel(w, /webhook secret token/i)
    expect(secret.attributes('placeholder')).toBe('********')
    expect((secret.element as HTMLInputElement).value).toBe('')
    expect(w.html()).not.toContain('test-webhook-secret')
    await fieldByLabel(w, /chat id/i).setValue('99')
    await w.get('form').trigger('submit')
    expect(connectorsNow().tg?.settings.webhook_token).toBe('test-webhook-secret')
    await button(/^edit tg$/i).trigger('click')
    await fieldByLabel(w, /webhook secret token/i).setValue('replacement-secret')
    await w.get('form').trigger('submit')
    expect(connectorsNow().tg?.settings.webhook_token).toBe('replacement-secret')
  })

  it('shows that a bot token is absent until one is saved, then displays its returned mask', async () => {
    const { w, button } = await mountCommunication({ tg: TELEGRAM })
    await button(/^edit tg$/i).trigger('click')
    expect(fieldByLabel(w, /^bot token/i).attributes('placeholder')).toBe('Not set')
    await fieldByLabel(w, /^bot token/i).setValue('123:replacement')
    await w.get('form').trigger('submit')
    await button(/^save/i).trigger('click')
    await flushPromises()
    await button(/^edit tg$/i).trigger('click')
    expect(fieldByLabel(w, /^bot token/i).attributes('placeholder')).toBe('••••1234')
    expect((fieldByLabel(w, /^bot token/i).element as HTMLInputElement).value).toBe('')
    expect(w.html()).not.toContain('123:replacement')
  })
})
