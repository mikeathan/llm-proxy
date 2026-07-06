async function handleResponse<T>(res: Response): Promise<T> {
  if (res.status === 204) {
    return {} as T
  }
  if (!res.ok) {
    const err = await res.json().catch(() => ({})) as Record<string, string>
    throw new Error(err['error'] || res.statusText)
  }
  return res.json() as Promise<T>
}

export async function get<T>(url: string): Promise<T> {
  const res = await fetch(url)
  return handleResponse<T>(res)
}

export async function post<T>(url: string, body?: unknown): Promise<T> {
  const res = await fetch(url, {
    method: 'POST',
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : {},
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  return handleResponse<T>(res)
}

export async function put<T>(url: string, body: unknown): Promise<T> {
  const res = await fetch(url, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  return handleResponse<T>(res)
}

export async function del<T = void>(url: string): Promise<T> {
  const res = await fetch(url, { method: 'DELETE' })
  return handleResponse<T>(res)
}
