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
