// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import type { ChipTone } from '@/components/ui/listFilters'
import type { CertCheckResult, CertMonitor, CertStatus } from '@/services/certificateApi'

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

/** Days left on a check, null when it never read an expiry date. */
export function checkDaysRemaining(check: CertCheckResult | undefined, days = check?.days_remaining): number | null {
  return check?.not_after && days != null ? days : null
}

export function certDaysRemaining(cert: CertMonitor): number | null {
  return checkDaysRemaining(cert.latest_check, cert.days_remaining ?? cert.latest_check?.days_remaining)
}

/** Days left before expiry, the number an operator scans a certificate list for. */
export function formatDaysRemaining(days: number | undefined | null): string {
  if (days === undefined || days === null) return 'Unknown'
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
