import { apiFetch } from './apiFetch'

const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'

export interface EditionResponse {
  edition: string
  features: Record<string, boolean>
}

export function fetchEdition(): Promise<EditionResponse> {
  return apiFetch(`${API_BASE}/edition`)
}
