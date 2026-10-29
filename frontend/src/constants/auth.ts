import type { TokenScope } from '../types/auth'

/** The shortest passphrase the server accepts (platform/auth.MinPassphraseLength). */
export const MIN_PASSPHRASE_LENGTH = 12
/** The longest token name the server accepts. */
export const TOKEN_NAME_MAX = 60

export const TOKEN_SCOPE_OPTIONS: { value: TokenScope; label: string }[] = [
  { value: 'admin', label: 'Admin API' },
  { value: 'inference', label: 'Model proxy only' },
]

/** The sign-in log's event kinds, in words. An unknown kind is shown as it is. */
export const EVENT_LABELS: Record<string, string> = {
  setup: 'Sign-in was set up',
  setup_code_issued: 'A setup code was made',
  setup_code_wrong: 'A wrong setup code was tried',
  login: 'Signed in',
  login_failed: 'A wrong passphrase was tried',
  logout: 'Signed out',
  reauth: 'Passphrase confirmed',
  reauth_failed: 'A wrong passphrase was tried to confirm',
  session_revoked: 'A session was ended',
  signed_out_everywhere: 'Signed out everywhere',
  passphrase_changed: 'Passphrase changed',
  passphrase_change_failed: 'A wrong passphrase was tried to change it',
  token_created: 'A script token was made',
  token_revoked: 'A script token was revoked',
  reset: 'Sign-in was reset from the server',
}
