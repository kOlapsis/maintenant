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

import type { ChipTone } from '@/components/ui/listFilters'
import type { Heartbeat } from '@/services/heartbeatApi'

const TONES: Record<Heartbeat['status'], ChipTone> = {
  up: 'ok',
  started: 'ok',
  down: 'down',
  paused: 'warn',
  new: 'neutral',
}

export function heartbeatTone(status: Heartbeat['status']): ChipTone {
  return TONES[status]
}

export function formatInterval(seconds: number): string {
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h`
  return `${Math.floor(seconds / 86400)}d`
}

/** Time left before the deadline, or how long it has been missed. */
export function formatDeadline(iso: string | undefined): string {
  if (!iso) return '-'
  const d = new Date(iso)
  if (isNaN(d.getTime()) || d.getFullYear() < 2000) return '-'
  const diff = Math.round((d.getTime() - Date.now()) / 1000)
  if (diff <= 0) return `overdue ${formatInterval(Math.abs(diff))}`
  return `in ${formatInterval(diff)}`
}
