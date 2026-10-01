// Product identity — the ONE place the product name lives (plan D25). The name
// is expected to change, so every wordmark, document title, aria-label and
// sentence imports it from here; index.html's <title> is filled from APP_TITLE
// at build time (vite.config.ts). A test fails on the literal name anywhere else.
// Storage keys, the backend binary and the config directory are out of scope:
// renaming those is a migration, not a copy change.

export const PRODUCT_NAME = 'llm-proxy'
export const APP_TITLE = `${PRODUCT_NAME} admin`
