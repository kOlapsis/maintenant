// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

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
