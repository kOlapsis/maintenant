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

const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'
import { apiFetch } from './apiFetch'

export interface UptimeDay {
  date: string
  uptime_percent: number | null
  incident_count: number
}

export function fetchEndpointDailyUptime(id: string, days = 90): Promise<UptimeDay[]> {
  return apiFetch<UptimeDay[]>(`${API_BASE}/endpoints/${id}/uptime/daily?days=${days}`)
}

export function fetchHeartbeatDailyUptime(id: string, days = 90): Promise<UptimeDay[]> {
  return apiFetch<UptimeDay[]>(`${API_BASE}/heartbeats/${id}/uptime/daily?days=${days}`)
}
