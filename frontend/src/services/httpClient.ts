const UNAUTHORIZED = 401
// The sign-in calls answer 401 for a wrong passphrase; that is their result, not a lost session.
const AUTH_PATH_PREFIX = '/admin/api/auth/'

type UnauthorizedHandler = () => void
/** Asks the user for their passphrase again; true when they gave it, so the request that needed it can be tried once more. */
type ReauthHandler = () => Promise<boolean>
let onUnauthorized: UnauthorizedHandler | null = null
let onReauthRequired: ReauthHandler | null = null

/** Registers what to do when the server says the user is not signed in (the app's one place that sends them to sign in). */
export function setUnauthorizedHandler(handler: UnauthorizedHandler | null): void {
  onUnauthorized = handler
}

/** Registers how to ask for the passphrase again when a destructive action needs it. */
export function setReauthHandler(handler: ReauthHandler | null): void {
  onReauthRequired = handler
}

/** For code that cannot see a status code (an EventSource) and has found out another way that the session ended. */
export function signalUnauthorized(): void {
  onUnauthorized?.()
}

const FORBIDDEN = 403
const REAUTH_REQUIRED = 'reauth_required'

const pathOf = (input: RequestInfo | URL): string => {
  const raw = typeof input === 'string' ? input : input instanceof URL ? input.pathname : input.url
  return raw.split('?')[0] ?? raw
}

// A body that is a string (or none) can be sent twice; a stream cannot.
const canResend = (init: RequestInit): boolean => init.body === undefined || init.body === null || typeof init.body === 'string'

async function needsPassphrase(res: Response): Promise<boolean> {
  if (res.status !== FORBIDDEN) return false
  const body = (await res.clone().json().catch(() => null)) as { error?: string } | null
  return body?.error === REAUTH_REQUIRED
}

/**
 * The one way the app sends a request. Every service goes through it (a lint rule forbids a bare `fetch`), so the page's
 * own credentials are always sent and a 401 is noticed in one place. A destructive action that needs the passphrase again
 * asks for it and is sent once more. The response is returned untouched.
 */
export async function apiFetch(input: RequestInfo | URL, init: RequestInit = {}): Promise<Response> {
  const res = await fetch(input, { ...init, credentials: 'same-origin' })
  if (res.status === UNAUTHORIZED && onUnauthorized && !pathOf(input).startsWith(AUTH_PATH_PREFIX)) onUnauthorized()
  if (onReauthRequired && canResend(init) && (await needsPassphrase(res)) && (await onReauthRequired())) {
    return fetch(input, { ...init, credentials: 'same-origin' })
  }
  return res
}

// A failed request. `field` is the setting a server-side validation refused, when the server says so; `status` is the HTTP
// status the server answered with (0 for an error made without a response).
export class HttpError extends Error {
  field: string
  status: number

  constructor(message: string, field = '', status = 0) {
    super(message)
    this.field = field
    this.status = status
  }
}

async function handleResponse<T>(res: Response): Promise<T> {
  if (res.status === 204) {
    return {} as T
  }
  if (!res.ok) {
    const err = await res.json().catch(() => ({})) as Record<string, string>
    throw new HttpError(err['error'] || res.statusText, err['field'], res.status)
  }
  return res.json() as Promise<T>
}

export async function get<T>(url: string): Promise<T> {
  const res = await apiFetch(url)
  return handleResponse<T>(res)
}

export async function post<T>(url: string, body?: unknown): Promise<T> {
  const res = await apiFetch(url, {
    method: 'POST',
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : {},
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  return handleResponse<T>(res)
}

export async function put<T>(url: string, body: unknown): Promise<T> {
  const res = await apiFetch(url, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  return handleResponse<T>(res)
}

export async function del<T = void>(url: string): Promise<T> {
  const res = await apiFetch(url, { method: 'DELETE' })
  return handleResponse<T>(res)
}
