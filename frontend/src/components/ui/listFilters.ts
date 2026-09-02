// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. See COMMERCIAL-LICENSE.md.

/**
 * Shared vocabulary for the list toolbar. Kept in a module rather than in the
 * component so plain .ts helpers can import it, the way statusGrid.ts does.
 */
export type ChipTone = 'ok' | 'warn' | 'down' | 'critical' | 'unknown' | 'neutral'

export interface StatusChip {
  value: string
  label: string
  count: number
  tone: ChipTone
}

/** Text colour and muted background for each tone. */
export const chipToneVars: Record<ChipTone, { fg: string; bg: string }> = {
  ok: { fg: 'var(--mnt-status-ok-text)', bg: 'var(--mnt-status-ok-bg)' },
  warn: { fg: 'var(--mnt-status-warn-text)', bg: 'var(--mnt-status-warn-bg)' },
  down: { fg: 'var(--mnt-status-down-text)', bg: 'var(--mnt-status-down-bg)' },
  critical: { fg: 'var(--mnt-status-critical-text)', bg: 'var(--mnt-status-critical-bg)' },
  unknown: { fg: 'var(--mnt-sev-unknown-text)', bg: 'var(--mnt-sev-unknown-bg)' },
  neutral: { fg: 'var(--mnt-text-muted)', bg: 'var(--mnt-bg-elevated)' },
}

/** Solid fill used by the status gutter down the left edge of a row. */
export const chipToneSolid: Record<ChipTone, string> = {
  ok: 'var(--mnt-status-ok)',
  warn: 'var(--mnt-status-warn)',
  down: 'var(--mnt-status-down)',
  critical: 'var(--mnt-status-critical)',
  unknown: 'var(--mnt-sev-unknown)',
  neutral: 'var(--mnt-border-default)',
}
