const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'
import { apiFetch, apiFetchVoid } from './apiFetch'

export interface WebhookSubscription {
  id: string
  name: string
  url: string
  event_types: string[]
  is_active: boolean
  last_delivery_status?: string
  last_delivery_at?: string
  failure_count: number
  created_at: string
}

export interface CreateWebhookInput {
  name: string
  url: string
  secret?: string
  event_types: string[]
}

export interface WebhooksResponse {
  webhooks: WebhookSubscription[]
}

export interface TestWebhookResponse {
  status: string
  http_status?: number
  error?: string
}

export function listWebhooks(): Promise<WebhooksResponse> {
  return apiFetch<WebhooksResponse>(`${API_BASE}/webhooks`)
}

export function createWebhook(data: CreateWebhookInput): Promise<WebhookSubscription> {
  return apiFetch<WebhookSubscription>(`${API_BASE}/webhooks`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export function deleteWebhook(id: string): Promise<void> {
  return apiFetchVoid(`${API_BASE}/webhooks/${id}`, { method: 'DELETE' })
}

export function testWebhook(id: string): Promise<TestWebhookResponse> {
  return apiFetch<TestWebhookResponse>(`${API_BASE}/webhooks/${id}/test`, {
    method: 'POST',
  })
}
