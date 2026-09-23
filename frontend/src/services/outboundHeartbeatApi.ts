// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { apiFetch, apiFetchVoid } from './apiFetch'

const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'

export interface OutboundHeartbeat {
  id: string
  name: string
  url: string
  interval_seconds: number
  enabled: boolean
  last_sent_at: string | null
  last_status_code: number | null
  last_error: string | null
  created_at: string
  updated_at: string
}

export interface OutboundHeartbeatsResponse {
  outbound_heartbeats: OutboundHeartbeat[]
}

export interface OutboundHeartbeatInput {
  name: string
  url: string
  interval_seconds: number
  enabled: boolean
}

export function listOutboundHeartbeats(): Promise<OutboundHeartbeatsResponse> {
  return apiFetch<OutboundHeartbeatsResponse>(`${API_BASE}/outbound-heartbeats`)
}

export function createOutboundHeartbeat(data: OutboundHeartbeatInput): Promise<OutboundHeartbeat> {
  return apiFetch<OutboundHeartbeat>(`${API_BASE}/outbound-heartbeats`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export function updateOutboundHeartbeat(
  id: string,
  data: OutboundHeartbeatInput,
): Promise<OutboundHeartbeat> {
  return apiFetch<OutboundHeartbeat>(`${API_BASE}/outbound-heartbeats/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export function deleteOutboundHeartbeat(id: string): Promise<void> {
  return apiFetchVoid(`${API_BASE}/outbound-heartbeats/${id}`, { method: 'DELETE' })
}

export function sendOutboundHeartbeat(id: string): Promise<OutboundHeartbeat> {
  return apiFetch<OutboundHeartbeat>(`${API_BASE}/outbound-heartbeats/${id}/send`, {
    method: 'POST',
  })
}
