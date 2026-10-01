import { APP_TITLE } from '../config/brand'

/** `(2) Activity · <app>` — the unread run-notification count prefixes the page title (plan D20, D25). */
export function documentTitle(pageTitle: string | undefined, unread: number): string {
  const base = pageTitle ? `${pageTitle} · ${APP_TITLE}` : APP_TITLE
  return unread > 0 ? `(${unread}) ${base}` : base
}
