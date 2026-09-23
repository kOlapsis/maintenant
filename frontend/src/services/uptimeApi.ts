// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'
import { apiFetch } from './apiFetch'

export interface UptimeDay {
  date: string
  uptime_percent: number | null
  incident_count: number
}

// The daily uptime endpoints wrap the series in an envelope; unwrap `days` so
// callers receive the bare array they expect.
interface DailyUptimeResponse {
  monitor_id: string
  monitor_type: string
  days: UptimeDay[]
}

export async function fetchEndpointDailyUptime(id: string, days = 90): Promise<UptimeDay[]> {
  const res = await apiFetch<DailyUptimeResponse>(`${API_BASE}/endpoints/${id}/uptime/daily?days=${days}`)
  return res.days ?? []
}

export async function fetchHeartbeatDailyUptime(id: string, days = 90): Promise<UptimeDay[]> {
  const res = await apiFetch<DailyUptimeResponse>(`${API_BASE}/heartbeats/${id}/uptime/daily?days=${days}`)
  return res.days ?? []
}

export async function fetchContainerDailyUptime(id: string, days = 90): Promise<UptimeDay[]> {
  const res = await apiFetch<DailyUptimeResponse>(`${API_BASE}/containers/${id}/uptime/daily?days=${days}`)
  return res.days ?? []
}
