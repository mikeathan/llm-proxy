// Names the backend accepts for workspaces and automations
// (handlers/dispatcher_handlers.go validIDRegex).
export const RESOURCE_NAME_PATTERN = /^[a-zA-Z0-9_-]{1,64}$/
export const RESOURCE_NAME_RULE = "Use 1–64 letters, digits, dashes and underscores."
