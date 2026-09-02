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
import type { CertStatus } from '@/services/certificateApi'

const TONES: Record<CertStatus, ChipTone> = {
  valid: 'ok',
  expiring: 'warn',
  expired: 'down',
  error: 'critical',
  unknown: 'unknown',
}

export function certificateTone(status: CertStatus): ChipTone {
  return TONES[status]
}

/** Days left before expiry, the number an operator scans a certificate list for. */
export function formatDaysRemaining(days: number | undefined | null): string {
  if (days === undefined || days === null) return '-'
  if (days < 0) return 'Expired'
  if (days === 0) return 'Today'
  return `${days}d`
}

export function countdownColor(days: number | undefined | null): string {
  if (days === undefined || days === null) return 'var(--mnt-text-muted)'
  if (days > 30) return 'var(--mnt-status-ok)'
  if (days > 7) return 'var(--mnt-status-warn)'
  if (days > 3) return 'var(--mnt-status-critical)'
  return 'var(--mnt-status-down)'
}

export function formatExpiryDate(iso: string | undefined): string {
  if (!iso) return '-'
  const d = new Date(iso)
  if (isNaN(d.getTime())) return '-'
  return d.toLocaleDateString(undefined, { day: '2-digit', month: 'short', year: 'numeric' })
}
