export function segKey(turnIdx: number, segIdx: number): string {
  return `${turnIdx}-${segIdx}`
}

export function toolLabel(name: string, args: string): string {
  if (!args || args === "{}") return name
  let parsed: any
  try { parsed = JSON.parse(args) } catch { return name }
  const arg = parsed.path || parsed.command || parsed.query || parsed.url || parsed.summary || ""
  if (typeof arg === "string" && arg.length > 40) return `${name}  ${arg.slice(0, 40)}…`
  if (arg) return `${name}  ${arg}`
  return name
}

export function toolIconClass(status: string): string {
  if (status === "running") return "tool-icon--running"
  if (status === "error") return "tool-icon--error"
  return "tool-icon--success"
}
