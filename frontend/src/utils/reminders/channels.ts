import type { ConnectorConfig } from '../../types/admin'
import { WEB_UI_LABEL } from '../../constants/reminders'

/** The connector types that are switched on, once each, in the order they were configured. */
export function enabledConnectorTypes(connectors: Record<string, ConnectorConfig> | undefined): string[] {
  return [...new Set(Object.values(connectors ?? {}).filter((c) => c.enabled).map((c) => c.type))]
}

/** The connector types a reminder that names no channel goes to: every enabled one that is not skipped. */
export function defaultConnectorTypes(enabled: string[], skipped: string[] | undefined): string[] {
  return enabled.filter((type) => !(skipped ?? []).includes(type))
}

/** "Web UI" or "Web UI + telegram, slack": where a reminder goes (the web UI always gets it). */
export function deliveryLabel(connectorTypes: string[]): string {
  return connectorTypes.length ? `${WEB_UI_LABEL} + ${connectorTypes.join(', ')}` : WEB_UI_LABEL
}

/** The skip list with one connector type ticked on or off; entries for other types are kept. */
export function withConnectorSkipped(skipped: string[] | undefined, type: string, receives: boolean): string[] {
  const rest = (skipped ?? []).filter((t) => t !== type)
  return receives ? rest : [...rest, type]
}
