const BASE = '/api/v1/status-page'

export interface PalettePayload {
  bg: string
  surface: string
  border: string
  text: string
  accent: string
  status_operational: string
  status_degraded: string
  status_partial: string
  status_major: string
}

export interface AnnouncementPayload {
  enabled: boolean
  message_md: string
  url: string
}

export interface SettingsPayload {
  title: string
  subtitle: string
  colors: PalettePayload
  announcement: AnnouncementPayload
  footer_text_md: string
  locale: string
  timezone: string
  date_format: string
}

export interface ContrastWarning {
  pair: string
  ratio: number
  wcag_aa_threshold: number
  severity: string
}

export interface SettingsResponse extends SettingsPayload {
  version: number
  announcement: AnnouncementPayload & { message_html: string }
  footer_text_html: string
  updated_at: string
  warnings?: { contrast: ContrastWarning[] }
}

export interface AssetMeta {
  role: string
  mime: string
  byte_size: number
  alt_text: string
  updated_at: string
}

export interface FooterLink {
  id: number
  position: number
  label: string
  url: string
  created_at: string
  updated_at: string
}

export interface FAQItem {
  id: number
  position: number
  question: string
  answer_md: string
  answer_html: string
  created_at: string
  updated_at: string
}

async function request<T>(method: string, url: string, body?: unknown): Promise<T> {
  const res = await fetch(url, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : {},
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: 'unknown_error' }))
    throw new Error(err?.error?.message || err?.message || `HTTP ${res.status}`)
  }
  if (res.status === 204) return undefined as T
  return res.json()
}

export const personalizationApi = {
  getSettings: () => request<SettingsResponse>('GET', `${BASE}/settings`),

  putSettings: (payload: SettingsPayload) =>
    request<SettingsResponse>('PUT', `${BASE}/settings`, payload),

  putAsset: async (role: string, file: File, altText?: string): Promise<AssetMeta> => {
    const form = new FormData()
    form.append('file', file)
    if (altText !== undefined) form.append('alt_text', altText)
    const res = await fetch(`${BASE}/assets/${role}`, { method: 'PUT', body: form })
    if (!res.ok) {
      const err = await res.json().catch(() => ({}))
      throw new Error(err?.error?.message || `HTTP ${res.status}`)
    }
    return res.json()
  },

  getAssetURL: (role: string) => `${BASE}/assets/${role}`,

  deleteAsset: (role: string) =>
    request<void>('DELETE', `${BASE}/assets/${role}`),

  listFooterLinks: () =>
    request<{ items: FooterLink[] }>('GET', `${BASE}/footer-links`),

  createFooterLink: (label: string, url: string) =>
    request<FooterLink>('POST', `${BASE}/footer-links`, { label, url }),

  updateFooterLink: (id: number, label: string, url: string) =>
    request<FooterLink>('PUT', `${BASE}/footer-links/${id}`, { label, url }),

  deleteFooterLink: (id: number) =>
    request<void>('DELETE', `${BASE}/footer-links/${id}`),

  reorderFooterLinks: (ids: number[]) =>
    request<{ items: FooterLink[] }>('PUT', `${BASE}/footer-links/order`, { ids }),

  listFAQ: () =>
    request<{ items: FAQItem[] }>('GET', `${BASE}/faq`),

  createFAQItem: (question: string, answer_md: string) =>
    request<FAQItem>('POST', `${BASE}/faq`, { question, answer_md }),

  updateFAQItem: (id: number, question: string, answer_md: string) =>
    request<FAQItem>('PUT', `${BASE}/faq/${id}`, { question, answer_md }),

  deleteFAQItem: (id: number) =>
    request<void>('DELETE', `${BASE}/faq/${id}`),

  reorderFAQ: (ids: number[]) =>
    request<{ items: FAQItem[] }>('PUT', `${BASE}/faq/order`, { ids }),
}
