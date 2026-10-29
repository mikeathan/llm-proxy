import type { ConnectorConfig } from "../types/admin"

const SECRET_BYTES = 32

// A random webhook secret: 64 hex characters, inside the 1–256 letters, digits, underscores and hyphens Telegram accepts.
export function newWebhookSecret(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(SECRET_BYTES))
  return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("")
}

// True for an enabled connector that is set up to receive messages (it names a workspace) and has no webhook secret:
// the server refuses every inbound call to it until it has one.
export function needsWebhookSecret(cfg: ConnectorConfig): boolean {
  return cfg.enabled && !!cfg.settings?.workspace_id && !cfg.settings?.webhook_token
}
