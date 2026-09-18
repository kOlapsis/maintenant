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

import type { Severity } from '@/composables/useSeverity'
import type {
  AgentOS,
  AgentOSSupport,
  OSSupportState,
  OSUnavailableReason,
} from '@/services/agentApi'
import type { HostOS, UpdateSummary } from '@/services/updateApi'

const STATE_LABELS: Record<OSSupportState, string> = {
  unknown: 'OS unknown',
  untracked: 'Not tracked',
  supported: 'Supported',
  security_only: 'Security updates only',
  ending_soon: 'Support ending soon',
  ended: 'Support ended',
}

const STATE_SEVERITIES: Record<OSSupportState, Severity> = {
  unknown: 'unknown',
  untracked: 'neutral',
  supported: 'ok',
  security_only: 'ok',
  ending_soon: 'warning',
  ended: 'incident',
}

const UNAVAILABLE_HINTS: Record<OSUnavailableReason, string> = {
  '': '',
  mount_missing: 'Mount /etc/os-release into the container',
  file_unreadable: '/etc/os-release unreadable on the host',
  node_not_found: 'Kubernetes node not found',
  agent_too_old: 'Update the agent',
}

export function osSupportLabel(state: OSSupportState): string {
  return STATE_LABELS[state] ?? STATE_LABELS.unknown
}

export function osSupportSeverity(state: OSSupportState): Severity {
  return STATE_SEVERITIES[state] ?? 'unknown'
}

export function osUnavailableHint(reason: OSUnavailableReason): string {
  return UNAVAILABLE_HINTS[reason] ?? ''
}

/** States the operator has to act on, or at least explain. */
export function osNeedsAttention(state: OSSupportState): boolean {
  return state === 'ended' || state === 'ending_soon' || state === 'unknown'
}

export function osDaysText(support: AgentOSSupport): string {
  const days = support.days_remaining
  if (days === null) return ''
  if (days < 0) return `ended ${Math.abs(days)} days ago`
  if (days === 0) return 'ends today'
  return `ends in ${days} days`
}

/** One line explaining the state: what to do about it, or when support runs out. */
export function osSupportHint(os: AgentOS): string {
  if (os.support.state === 'unknown') {
    return osUnavailableHint(os.unavailable_reason) || 'The agent has not reported an OS identity'
  }
  if (os.support.state === 'untracked') {
    return 'No end-of-life dates published for this distribution'
  }
  const until = os.support.security_until ?? os.support.active_until
  const days = osDaysText(os.support)
  if (!until) return osSupportLabel(os.support.state)
  return days ? `Security support ${days} (${until})` : `Security support until ${until}`
}

const PRODUCT_LABELS: Record<string, string> = {
  debian: 'Debian',
  ubuntu: 'Ubuntu',
  rhel: 'RHEL',
  'rocky-linux': 'Rocky Linux',
  almalinux: 'AlmaLinux',
  'alpine-linux': 'Alpine',
  sles: 'SLES',
}

const EXTENDED_NAMES: Record<string, string> = {
  debian: 'ELTS',
  ubuntu: 'ESM',
  rhel: 'ELS',
  sles: 'LTSS',
}

const STATE_ORDER: Record<OSSupportState, number> = {
  ended: 0,
  ending_soon: 1,
  security_only: 2,
  supported: 3,
  untracked: 4,
  unknown: 5,
}

export function osCanonicalName(support: AgentOSSupport): string {
  if (!support.product) return ''
  const label = PRODUCT_LABELS[support.product] ?? support.product
  return support.cycle ? `${label} ${support.cycle}` : label
}

export interface OSGroup {
  key: string
  state: OSSupportState
  name: string
  prettyName: string
  support: AgentOSSupport
  hosts: HostOS[]
  hints: string[]
}

function hostDisplayName(host: HostOS): string {
  return host.label || host.hostname
}

function isTracked(state: OSSupportState): boolean {
  return state !== 'unknown' && state !== 'untracked'
}

function groupKey(host: HostOS): string {
  const { support } = host.os
  if (support.state === 'unknown') return 'unknown'
  if (isTracked(support.state) && support.product) return `eol:${support.product}:${support.cycle ?? ''}`
  return `os:${host.os.pretty_name || `${host.os.id} ${host.os.version_id}`.trim()}`
}

function unknownHints(hosts: HostOS[]): string[] {
  const counts = new Map<string, number>()
  for (const host of hosts) {
    const hint = osUnavailableHint(host.os.unavailable_reason) || 'The agent has not reported an OS identity'
    counts.set(hint, (counts.get(hint) ?? 0) + 1)
  }
  return [...counts.entries()].sort((a, b) => b[1] - a[1]).map(([hint]) => hint)
}

/** Groups hosts by OS version, most urgent first, hosts without an OS identity last. */
export function groupHostsByOS(hosts: HostOS[]): OSGroup[] {
  const byKey = new Map<string, OSGroup>()
  for (const host of hosts) {
    const key = groupKey(host)
    const group = byKey.get(key)
    if (group) {
      group.hosts.push(host)
      continue
    }
    const { support } = host.os
    const prettyName = host.os.pretty_name || `${host.os.id} ${host.os.version_id}`.trim()
    byKey.set(key, {
      key,
      state: support.state,
      name: osCanonicalName(support) || prettyName,
      prettyName,
      support,
      hosts: [host],
      hints: [],
    })
  }

  const groups = [...byKey.values()]
  for (const group of groups) {
    group.hosts.sort((a, b) => hostDisplayName(a).localeCompare(hostDisplayName(b)))
    if (group.state === 'unknown') group.hints = unknownHints(group.hosts)
  }

  return groups.sort((a, b) => {
    const byState = STATE_ORDER[a.state] - STATE_ORDER[b.state]
    if (byState !== 0) return byState
    const byDays = (a.support.days_remaining ?? 0) - (b.support.days_remaining ?? 0)
    if (byDays !== 0) return byDays
    return a.name.localeCompare(b.name, undefined, { numeric: true })
  })
}

function spanText(days: number): string {
  const months = Math.floor(days / 30.44)
  const years = Math.floor(months / 12)
  const rest = months % 12
  if (years === 0) return `${rest} mo`
  return rest === 0 ? `${years} y` : `${years} y ${rest} mo`
}

function daysWord(days: number): string {
  return days === 1 ? '1 day' : `${days} days`
}

/** Time left before security support ends, in days when close and in years and months when far. */
export function osTimeLeft(support: AgentOSSupport): string {
  const days = support.days_remaining
  if (days === null) return ''
  if (days === 0) return 'Ends today'
  if (days < 0) {
    const past = -days
    return past < 60 ? `Ended ${daysWord(past)} ago` : `Ended ${spanText(past)} ago`
  }
  return days < 60 ? `Ends in ${daysWord(days)}` : `${spanText(days)} left`
}

export function osStateTextClass(state: OSSupportState): string {
  switch (state) {
    case 'ended': return 'text-mnt-status-down'
    case 'ending_soon': return 'text-mnt-status-warn'
    case 'supported':
    case 'security_only': return 'text-mnt-status-ok'
    default: return 'text-mnt-muted'
  }
}

export type OSPhaseStatus = 'done' | 'current' | 'upcoming'

export interface OSPhase {
  key: 'active' | 'security' | 'extended'
  label: string
  until: string
  status: OSPhaseStatus
  paid: boolean
}

function localDay(date: Date): string {
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${date.getFullYear()}-${month}-${day}`
}

/** The support phases of an OS version with their end dates, relative to today. */
export function osPhases(support: AgentOSSupport, today: Date = new Date()): OSPhase[] {
  const now = localDay(today)
  const candidates: Array<Omit<OSPhase, 'status'>> = []
  if (support.active_until) {
    candidates.push({ key: 'active', label: 'Active', until: support.active_until, paid: false })
  }
  if (support.security_until) {
    candidates.push({ key: 'security', label: 'Security', until: support.security_until, paid: false })
  }
  if (support.extended_until) {
    const name = (support.product && EXTENDED_NAMES[support.product]) || 'extended'
    candidates.push({ key: 'extended', label: `Paid ${name}`, until: support.extended_until, paid: true })
  }

  let currentFound = false
  return candidates.map((phase) => {
    let status: OSPhaseStatus
    if (phase.until < now) status = 'done'
    else if (!currentFound && !phase.paid) {
      status = 'current'
      currentFound = true
    } else status = 'upcoming'
    return { ...phase, status }
  })
}

type OSCounts = UpdateSummary['os_counts']

export function osAtRisk(counts: OSCounts): number {
  return counts.ended + counts.ending_soon
}

export function osTotal(counts: OSCounts): number {
  return counts.ended + counts.ending_soon + counts.unknown + counts.untracked + counts.supported
}

export function osAtRiskTone(counts: OSCounts): string {
  if (counts.ended > 0) return 'text-mnt-status-down'
  if (counts.ending_soon > 0) return 'text-mnt-status-warn'
  return 'text-mnt-muted'
}
