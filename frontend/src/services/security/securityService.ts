import { API_ENDPOINTS } from '../../constants/api'
import { del, get, post } from '../httpClient'
import type { AgentWatchView, LockdownState } from '../../types/security'

/** The agent security record and the lock down switch. Switching lock down on or off asks for the passphrase again. */
export class SecurityService {
  static agentWatch(): Promise<AgentWatchView> {
    return get<AgentWatchView>(API_ENDPOINTS.agentWatch)
  }

  static lockdown(): Promise<LockdownState> {
    return get<LockdownState>(API_ENDPOINTS.lockdown)
  }

  static engageLockdown(revokeTokens: boolean): Promise<LockdownState> {
    return post<LockdownState>(API_ENDPOINTS.lockdown, { revoke_tokens: revokeTokens })
  }

  static releaseLockdown(): Promise<LockdownState> {
    return del<LockdownState>(API_ENDPOINTS.lockdown)
  }
}
