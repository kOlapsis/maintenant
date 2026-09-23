// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { apiFetch, apiFetchVoid } from './apiFetch'
import type { AlertTrigger, TriggerRequest } from '@/types/triggers'

const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'

export function listTriggers(): Promise<{ triggers: AlertTrigger[] }> {
  return apiFetch(`${API_BASE}/alert-triggers`)
}

export function getTrigger(id: string): Promise<AlertTrigger> {
  return apiFetch(`${API_BASE}/alert-triggers/${id}`)
}

export function createTrigger(data: TriggerRequest): Promise<AlertTrigger> {
  return apiFetch(`${API_BASE}/alert-triggers`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export function updateTrigger(id: string, data: TriggerRequest): Promise<AlertTrigger> {
  return apiFetch(`${API_BASE}/alert-triggers/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export function deleteTrigger(id: string): Promise<void> {
  return apiFetchVoid(`${API_BASE}/alert-triggers/${id}`, { method: 'DELETE' })
}
