/**
 * Shared fetch wrapper for all API services.
 */

export async function apiFetch<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init)

  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body?.error?.message || `HTTP ${res.status}`)
  }

  return res.json()
}

export async function apiFetchVoid(url: string, init?: RequestInit): Promise<void> {
  const res = await fetch(url, init)

  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body?.error?.message || `HTTP ${res.status}`)
  }
}
