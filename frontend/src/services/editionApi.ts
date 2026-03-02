import { apiFetch } from './apiFetch'

const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'

export interface EditionResponse {
  edition: string
  organisation_name: string
  features: Record<string, boolean>
}

export function fetchEdition(): Promise<EditionResponse> {
  return apiFetch(`${API_BASE}/edition`)
}

export interface LicenseStatus {
  status: string
  plan: string
  message: string
  verified_at: string
  expires_at: string
}

export function fetchLicenseStatus(): Promise<LicenseStatus> {
  return apiFetch(`${API_BASE}/license/status`)
}
