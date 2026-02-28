const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'
import { apiFetch } from './apiFetch'

export interface UptimeDay {
  date: string
  uptime_percent: number | null
  incident_count: number
}

export function fetchEndpointDailyUptime(id: number, days = 90): Promise<UptimeDay[]> {
  return apiFetch<UptimeDay[]>(`${API_BASE}/endpoints/${id}/uptime/daily?days=${days}`)
}

export function fetchHeartbeatDailyUptime(id: number, days = 90): Promise<UptimeDay[]> {
  return apiFetch<UptimeDay[]>(`${API_BASE}/heartbeats/${id}/uptime/daily?days=${days}`)
}
