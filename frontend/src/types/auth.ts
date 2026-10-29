// Sign-in (plan admin-api-authentication): the shapes the /admin/api/auth endpoints return and take.

/** What a script token may do: reach the admin API, or only the model proxy (/v1). */
export type TokenScope = 'admin' | 'inference'

/** What anyone may ask before signing in. */
export interface AuthStatus {
  /** A passphrase exists, so every admin request needs a credential. */
  required: boolean
  /** A setup code was made and has not expired, so the first passphrase can be set here. */
  setup_open: boolean
  /** This browser is signed in. */
  authenticated: boolean
  /** This browser re-entered the passphrase recently enough for the destructive actions. */
  elevated: boolean
  /** The connection is HTTPS. */
  encrypted: boolean
}

/** A browser signed in. The id names it for revoking; it is not the cookie. */
export interface AuthSession {
  id: string
  address?: string
  browser?: string
  created_at: string
  last_seen: string
  current: boolean
}

/** A script token as listed: never its secret. */
export interface AuthToken {
  id: string
  name: string
  scope: TokenScope
  created_at: string
  last_used?: string
}

/** What creating a token answers: the secret, shown once. */
export interface NewAuthToken {
  id: string
  name: string
  scope: TokenScope
  token: string
}

/** One line of the sign-in log. It never holds a secret. */
export interface AuthEvent {
  at: string
  kind: string
  address?: string
  detail?: string
}

/** One thing about the setup that needs attention, with what to do about it. */
export interface ExposureAttention {
  id: string
  text: string
  fix: string
}

/** Where the server listens and what protects it (the Sign-in page's Exposure card). It holds no secret. */
export interface AuthExposure {
  listening: { address: string; /** Other machines can connect. */ network: boolean }
  sign_in_required: boolean
  /** The connection this page came over is HTTPS. */
  encrypted: boolean
  inference_key_required: boolean
  webhooks: { inbound: number; with_secret: number }
  sandbox: { enabled: boolean; /** The file confinement agents run under, or "none". */ filesystem: string }
  attention: ExposureAttention[]
}

/** How long a sign-in lasts: the idle limit in hours and the absolute limit in days, with the defaults and the ceilings. */
export interface SessionLength {
  idle_hours: number
  absolute_days: number
  max_idle_hours: number
  max_absolute_days: number
  default_idle_hours: number
  default_absolute_days: number
}
