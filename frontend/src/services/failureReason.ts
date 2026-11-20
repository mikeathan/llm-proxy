// The backend answers errors as {"error": "…"}; show the reason, not the JSON.
export async function failureReason(res: Response, what: string): Promise<Error> {
  const text = await res.text()
  try {
    const reason = (JSON.parse(text) as { error?: string }).error
    if (reason) return new Error(reason)
  } catch {
    // not JSON: fall through to the raw text
  }
  return new Error(`Failed to ${what}: ${res.status} - ${text}`)
}
