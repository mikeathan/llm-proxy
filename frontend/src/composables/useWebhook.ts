import { ref } from "vue"
import type { Ref } from "vue"
import type { ConnectorConfig, WebhookUIState } from "../types/admin"
import { AdminApiService } from "../services/admin/adminService"

// Singleton state — shared across consumers (per AGENTS.md composable convention).
// Only CommunicationSettings consumes this, so single-instance is correct.
const webhookUI = ref<Record<string, WebhookUIState>>({})
const webhookBaseUrl = ref(window.location.origin + "/api/v1/webhooks/")
const defaultHost = ref(window.location.host)

export function useWebhook(
  connectors: Ref<Record<string, ConnectorConfig>>,
  saveError: Ref<string>,
) {
  function webhookState(name: string): WebhookUIState {
    if (!webhookUI.value[name]) {
      webhookUI.value[name] = {
        host: "", info: null, creating: false, verifying: false, deleting: false,
        verifyState: "idle", verifyMsg: "", statusMsg: "",
      }
    }
    return webhookUI.value[name]
  }

  // Clones connectors to trigger the computed setter (which emits update:editConfig),
  // persisting the webhook URL in registry.json via the settings save flow.
  function setConnectorWebhookUrl(name: string, url: string | undefined) {
    const updated = { ...connectors.value }
    if (updated[name]) {
      updated[name] = { ...updated[name], webhook_url: url }
      connectors.value = updated
    }
  }

  // Builds the full webhook URL from user input. Accepts a bare hostname (tunnel
  // endpoint like "my-tunnel.ngrok.io") or a full URL with scheme.
  function normalizeWebhookUrl(name: string, input: string): string {
    const base = webhookBaseUrl.value + name
    if (!input) return base
    const trimmed = input.trim()
    if (trimmed.startsWith("http://") || trimmed.startsWith("https://")) {
      return trimmed + (trimmed.endsWith("/") ? "" : "/") + "api/v1/webhooks/" + name
    }
    // bare hostname — construct full URL
    const host = trimmed.replace(/\/+$/, "")
    return `https://${host}/api/v1/webhooks/${name}`
  }

  async function createWebhook(name: string) {
    if (webhookState(name).creating) return
    webhookState(name).creating = true
    saveError.value = ""
    webhookState(name).statusMsg = ""
    try {
      const hostInput = webhookState(name).host ?? ""
      const fullUrl = normalizeWebhookUrl(name, hostInput)
      await AdminApiService.createConnectorWebhook(name, fullUrl)
      setConnectorWebhookUrl(name, fullUrl)
      webhookState(name).statusMsg = "Webhook created"
      await verifyWebhook(name)
    } catch (err) {
      saveError.value = `Failed to create webhook for "${name}": ${(err as Error).message}`
    } finally {
      webhookState(name).creating = false
    }
  }

  async function verifyWebhook(name: string) {
    if (webhookState(name).verifying) return
    webhookState(name).verifying = true
    webhookState(name).verifyState = "checking"
    webhookState(name).verifyMsg = ""
    saveError.value = ""
    try {
      webhookState(name).info = await AdminApiService.verifyConnectorWebhook(name)
      const info = webhookState(name).info
      if (info?.url && !info.last_error) {
        webhookState(name).verifyState = "registered"
        webhookState(name).verifyMsg = info.pending_updates > 0 ? `${info.pending_updates} pending` : "Live"
      } else if (info?.url && info.last_error) {
        webhookState(name).verifyState = "registered"
        webhookState(name).verifyMsg = `Error: ${info.last_error}`
      } else {
        webhookState(name).verifyState = "unregistered"
        webhookState(name).verifyMsg = "No webhook registered at Telegram"
      }
    } catch (err) {
      webhookState(name).verifyState = "error"
      webhookState(name).verifyMsg = (err as Error).message
      saveError.value = `Failed to verify webhook for "${name}": ${(err as Error).message}`
    } finally {
      webhookState(name).verifying = false
    }
  }

  async function deleteWebhook(name: string) {
    if (webhookState(name).deleting) return
    webhookState(name).deleting = true
    saveError.value = ""
    webhookState(name).statusMsg = ""
    try {
      await AdminApiService.deleteConnectorWebhook(name)
      webhookState(name).info = null
      setConnectorWebhookUrl(name, undefined)
      webhookState(name).verifyState = "unregistered"
      webhookState(name).verifyMsg = ""
      webhookState(name).statusMsg = "Webhook deleted"
    } catch (err) {
      saveError.value = `Failed to delete webhook for "${name}": ${(err as Error).message}`
    } finally {
      webhookState(name).deleting = false
    }
  }

  function clearWebhookState(name: string) {
    delete webhookUI.value[name]
  }

  return {
    webhookBaseUrl,
    defaultHost,
    webhookState,
    createWebhook,
    verifyWebhook,
    deleteWebhook,
    clearWebhookState,
  }
}
