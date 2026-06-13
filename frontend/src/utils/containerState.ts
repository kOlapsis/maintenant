/*
 * Copyright 2026 Benjamin Touchard (kOlapsis)
 *
 * Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
 * or a commercial license. You may not use this file except in compliance
 * with one of these licenses.
 *
 * AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
 * Commercial: See COMMERCIAL-LICENSE.md
 *
 * Source: https://github.com/kolapsis/maintenant
 */

export interface StateStyle {
  color: string
  bg: string
  glow?: string
}

const stateStyles: Record<string, StateStyle> = {
  running: { color: 'var(--mnt-status-ok)', bg: 'var(--mnt-status-ok-bg)', glow: 'var(--mnt-glow-ok)' },
  exited: { color: 'var(--mnt-status-down)', bg: 'var(--mnt-status-down-bg)' },
  completed: { color: 'var(--mnt-text-secondary)', bg: 'var(--mnt-bg-elevated)' },
  restarting: { color: 'var(--mnt-status-warn)', bg: 'var(--mnt-status-warn-bg)', glow: 'var(--mnt-glow-warn)' },
  paused: { color: 'var(--mnt-accent)', bg: 'var(--mnt-bg-elevated)' },
  created: { color: 'var(--mnt-text-muted)', bg: 'var(--mnt-bg-elevated)' },
  dead: { color: 'var(--mnt-status-down)', bg: 'var(--mnt-status-down-bg)' },
}

const defaultStyle: StateStyle = { color: 'var(--mnt-text-muted)', bg: 'var(--mnt-bg-elevated)' }

export function getStateStyle(state: string): StateStyle {
  return stateStyles[state] ?? defaultStyle
}

export function getStateColor(state: string): string {
  return (stateStyles[state] ?? defaultStyle).color
}

/** Exit codes that indicate a graceful/voluntary stop, not a crash. */
const gracefulExitCodes = new Set([0, 137, 143])

export function isGracefulExitCode(code: number): boolean {
  return gracefulExitCodes.has(code)
}

export interface ExitCodeStyle {
  bg: string
  color: string
}

export function getExitCodeStyle(code: number): ExitCodeStyle {
  if (isGracefulExitCode(code)) {
    return { bg: 'var(--mnt-bg-elevated)', color: 'var(--mnt-text-secondary)' }
  }
  return { bg: 'var(--mnt-status-down-bg)', color: 'var(--mnt-status-down)' }
}
