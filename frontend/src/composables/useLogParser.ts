/*
  Copyright 2026 Benjamin Touchard (kOlapsis)

  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. You may not use this file except in compliance
  with one of these licenses.

  AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
  Commercial: See COMMERCIAL-LICENSE.md

  Source: https://github.com/kolapsis/maintenant
*/

import type { LogLevel } from './useLogStream'

// ── ANSI stripping (for JSON detection) ──────────────────────────────

const ANSI_RE = /\x1b\[[^a-zA-Z]*[a-zA-Z]/g

function stripAnsi(s: string): string {
  return s.replace(ANSI_RE, '')
}

// ── Log level detection ──────────────────────────────────────────────

// Patterns ordered by severity. First match (left-to-right in the line) wins.
// Each entry: [regex, level]. Regexes are case-insensitive.
const LEVEL_PATTERNS: [RegExp, LogLevel][] = [
  // key=value: level=FATAL, lvl=fatal, severity=critical
  [/(?:level|lvl|severity)\s*=\s*"?(?:fatal|critical)"?/i, 'fatal'],
  // JSON: "level":"fatal", "severity":"critical"
  [/"(?:level|severity)"\s*:\s*"(?:fatal|critical)"/i, 'fatal'],
  // Bracketed: [FATAL], [CRITICAL]
  [/\[(?:FATAL|CRITICAL)\]/i, 'fatal'],
  // Colon-suffix at word boundary: FATAL: or CRITICAL:
  [/\b(?:FATAL|CRITICAL):/i, 'fatal'],

  // ERROR
  [/(?:level|lvl|severity)\s*=\s*"?(?:error|err)"?/i, 'error'],
  [/"(?:level|severity)"\s*:\s*"(?:error|err)"/i, 'error'],
  [/\[(?:ERROR|ERR)\]/i, 'error'],
  [/\b(?:ERROR|ERR):/i, 'error'],

  // WARN
  [/(?:level|lvl|severity)\s*=\s*"?(?:warn(?:ing)?)"?/i, 'warn'],
  [/"(?:level|severity)"\s*:\s*"(?:warn(?:ing)?)"/i, 'warn'],
  [/\[WARN(?:ING)?\]/i, 'warn'],
  [/\bWARN(?:ING)?:/i, 'warn'],

  // INFO
  [/(?:level|lvl|severity)\s*=\s*"?(?:info|inf)"?/i, 'info'],
  [/"(?:level|severity)"\s*:\s*"(?:info|inf)"/i, 'info'],
  [/\[(?:INFO|INF)\]/i, 'info'],
  [/\b(?:INFO|INF):/i, 'info'],

  // DEBUG
  [/(?:level|lvl|severity)\s*=\s*"?(?:debug|dbg)"?/i, 'debug'],
  [/"(?:level|severity)"\s*:\s*"(?:debug|dbg)"/i, 'debug'],
  [/\[(?:DEBUG|DBG)\]/i, 'debug'],
  [/\b(?:DEBUG|DBG):/i, 'debug'],

  // TRACE
  [/(?:level|lvl|severity)\s*=\s*"?(?:trace|trc)"?/i, 'trace'],
  [/"(?:level|severity)"\s*:\s*"(?:trace|trc)"/i, 'trace'],
  [/\[(?:TRACE|TRC)\]/i, 'trace'],
  [/\b(?:TRACE|TRC):/i, 'trace'],
]

export function detectLogLevel(raw: string): LogLevel {
  let bestLevel: LogLevel = 'unknown'
  let bestIndex = Infinity

  for (const [re, level] of LEVEL_PATTERNS) {
    re.lastIndex = 0
    const m = re.exec(raw)
    if (m && m.index < bestIndex) {
      bestIndex = m.index
      bestLevel = level
    }
  }

  return bestLevel
}

// ── JSON detection ───────────────────────────────────────────────────

const TIMESTAMP_PREFIX_RE = /^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?\s+/

export function parseJsonLine(raw: string): { json: Record<string, unknown>; prefix: string } | null {
  const cleaned = stripAnsi(raw)

  // Try direct JSON first (line starts with {)
  if (cleaned.trimStart().startsWith('{')) {
    const trimmed = cleaned.trimStart()
    const prefix = cleaned.slice(0, cleaned.length - trimmed.length)
    try {
      const json = JSON.parse(trimmed) as Record<string, unknown>
      if (typeof json === 'object' && json !== null && !Array.isArray(json)) {
        return { json, prefix }
      }
    } catch {
      // Not valid JSON
    }
  }

  // Try after stripping timestamp prefix
  const tsMatch = TIMESTAMP_PREFIX_RE.exec(cleaned)
  if (tsMatch) {
    const after = cleaned.slice(tsMatch[0].length)
    if (after.trimStart().startsWith('{')) {
      const trimmed = after.trimStart()
      const prefix = cleaned.slice(0, cleaned.length - trimmed.length)
      try {
        const json = JSON.parse(trimmed) as Record<string, unknown>
        if (typeof json === 'object' && json !== null && !Array.isArray(json)) {
          return { json, prefix }
        }
      } catch {
        // Not valid JSON
      }
    }
  }

  return null
}

// ── Timestamp extraction ─────────────────────────────────────────────

const ISO_TS_RE = /^(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?)/
const SYSLOG_TS_RE = /^([A-Z][a-z]{2}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2})/

export function parseTimestamp(raw: string): Date | null {
  const cleaned = stripAnsi(raw).trimStart()

  const isoMatch = ISO_TS_RE.exec(cleaned)
  if (isoMatch) {
    const d = new Date(isoMatch[1]!)
    if (!isNaN(d.getTime())) return d
  }

  const syslogMatch = SYSLOG_TS_RE.exec(cleaned)
  if (syslogMatch) {
    const year = new Date().getFullYear()
    const d = new Date(`${syslogMatch[1]} ${year}`)
    if (!isNaN(d.getTime())) return d
  }

  return null
}
