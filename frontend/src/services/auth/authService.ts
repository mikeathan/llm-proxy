import { API_ENDPOINTS } from '../../constants/api'
import { del, get, post, put } from '../httpClient'
import type { AuthEvent, AuthExposure, AuthSession, SessionLength, AuthStatus, AuthToken, NewAuthToken, TokenScope } from '../../types/auth'

const query = (params: Record<string, string>) => `?${new URLSearchParams(params).toString()}`

export class AuthService {
  static status(): Promise<AuthStatus> {
    return get<AuthStatus>(API_ENDPOINTS.authStatus)
  }

  /** Sets the first passphrase from the one-time setup code, and signs in. */
  static setup(code: string, passphrase: string): Promise<unknown> {
    return post(API_ENDPOINTS.authSetup, { code, passphrase })
  }

  static login(passphrase: string): Promise<unknown> {
    return post(API_ENDPOINTS.authLogin, { passphrase })
  }

  static logout(): Promise<unknown> {
    return post(API_ENDPOINTS.authLogout)
  }

  /** Confirms the passphrase again, opening a short window for the destructive actions. */
  static reauth(passphrase: string): Promise<unknown> {
    return post(API_ENDPOINTS.authReauth, { passphrase })
  }

  static sessions(): Promise<AuthSession[]> {
    return get<AuthSession[]>(API_ENDPOINTS.authSessions)
  }

  static revokeSession(id: string): Promise<void> {
    return del(`${API_ENDPOINTS.authSessions}${query({ id })}`)
  }

  static revokeAllSessions(): Promise<unknown> {
    return post(API_ENDPOINTS.authSessionsRevokeAll)
  }

  static changePassphrase(oldPassphrase: string, newPassphrase: string): Promise<unknown> {
    return post(API_ENDPOINTS.authPassphrase, { old: oldPassphrase, new: newPassphrase })
  }

  static tokens(): Promise<AuthToken[]> {
    return get<AuthToken[]>(API_ENDPOINTS.authTokens)
  }

  static createToken(name: string, scope: TokenScope): Promise<NewAuthToken> {
    return post<NewAuthToken>(API_ENDPOINTS.authTokens, { name, scope })
  }

  static revokeToken(id: string): Promise<void> {
    return del(`${API_ENDPOINTS.authTokens}${query({ id })}`)
  }

  static events(): Promise<AuthEvent[]> {
    return get<AuthEvent[]>(API_ENDPOINTS.authEvents)
  }

  static sessionLength(): Promise<SessionLength> {
    return get<SessionLength>(API_ENDPOINTS.authSessionLength)
  }

  /** Saves the two limits. A longer session weakens sign-in, so the server asks for the passphrase again first. */
  static setSessionLength(idleHours: number, absoluteDays: number): Promise<SessionLength> {
    return put<SessionLength>(API_ENDPOINTS.authSessionLength, { idle_hours: idleHours, absolute_days: absoluteDays })
  }

  static exposure(): Promise<AuthExposure> {
    return get<AuthExposure>(API_ENDPOINTS.authExposure)
  }
}
