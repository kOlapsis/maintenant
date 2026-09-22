// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. See COMMERCIAL-LICENSE.md.

import { describe, it, expect } from 'vitest'
import type { AgentOSSupport, OSSupportState, OSUnavailableReason } from '@/services/agentApi'
import type { HostOS } from '@/services/updateApi'
import { groupHostsByOS, osAtRisk, osPhases, osTimeLeft, osTotal } from '../osSupport'

function support(overrides: Partial<AgentOSSupport> = {}): AgentOSSupport {
  return {
    state: 'supported',
    product: null,
    cycle: null,
    active_until: null,
    security_until: null,
    extended_until: null,
    days_remaining: null,
    table_source: 'embedded',
    ...overrides,
  }
}

function host(
  id: string,
  state: OSSupportState,
  overrides: Partial<AgentOSSupport> = {},
  prettyName = '',
  reason: OSUnavailableReason = '',
): HostOS {
  return {
    agent_id: id,
    hostname: `${id}.lan`,
    label: '',
    is_local: false,
    runtime: 'docker',
    connection_state: 'connected',
    os: {
      id: '',
      version_id: '',
      pretty_name: prettyName,
      source: 'host_file',
      unavailable_reason: reason,
      reported_at: null,
      support: support({ state, ...overrides }),
    },
  }
}

describe('groupHostsByOS', () => {
  const hosts = [
    host('h-unknown-1', 'unknown', {}, '', 'mount_missing'),
    host('h-bookworm', 'supported', { product: 'debian', cycle: '12', days_remaining: 700 }),
    host('h-bullseye-b', 'ended', { product: 'debian', cycle: '11', days_remaining: -18 }, 'Debian GNU/Linux 11 (bullseye)'),
    host('h-untracked', 'untracked', {}, 'Arch Linux'),
    host('h-buster', 'ended', { product: 'debian', cycle: '10', days_remaining: -800 }),
    host('h-jammy', 'ending_soon', { product: 'ubuntu', cycle: '22.04', days_remaining: 12 }),
    host('h-bullseye-a', 'ended', { product: 'debian', cycle: '11', days_remaining: -18 }),
    host('h-unknown-2', 'unknown', {}, '', 'agent_too_old'),
    host('h-unknown-3', 'unknown', {}, '', 'agent_too_old'),
  ]

  it('groups hosts by product and cycle', () => {
    const bullseye = groupHostsByOS(hosts).find((g) => g.name === 'Debian 11')
    expect(bullseye?.hosts.map((h) => h.agent_id)).toEqual(['h-bullseye-a', 'h-bullseye-b'])
  })

  it('orders ended oldest first, then ending soon, supported, untracked, unknown last', () => {
    expect(groupHostsByOS(hosts).map((g) => g.name)).toEqual([
      'Debian 10',
      'Debian 11',
      'Ubuntu 22.04',
      'Debian 12',
      'Arch Linux',
      '',
    ])
  })

  it('collapses unknown hosts into one group with hints by frequency', () => {
    const groups = groupHostsByOS(hosts)
    const unknown = groups[groups.length - 1]
    expect(unknown?.state).toBe('unknown')
    expect(unknown?.hosts).toHaveLength(3)
    expect(unknown?.hints).toEqual(['Update the agent', 'Mount /etc/os-release into the container'])
  })
})

describe('osTimeLeft', () => {
  it.each([
    [null, ''],
    [-18, 'Ended 18 days ago'],
    [-1, 'Ended 1 day ago'],
    [-400, 'Ended 1 y 1 mo ago'],
    [0, 'Ends today'],
    [12, 'Ends in 12 days'],
    [335, '11 mo left'],
    [1705, '4 y 8 mo left'],
    [731, '2 y left'],
  ])('days_remaining %s gives %s', (days, text) => {
    expect(osTimeLeft(support({ days_remaining: days }))).toBe(text)
  })
})

describe('osPhases', () => {
  const debian11 = support({
    state: 'ended',
    product: 'debian',
    active_until: '2024-08-14',
    security_until: '2026-08-31',
    extended_until: '2031-06-30',
  })

  it('marks past phases done and never makes the paid phase current', () => {
    expect(osPhases(debian11, new Date(2026, 8, 18)).map((p) => [p.label, p.status])).toEqual([
      ['Active', 'done'],
      ['Security', 'done'],
      ['Paid ELTS', 'upcoming'],
    ])
  })

  it('makes the first unfinished free phase current', () => {
    expect(osPhases(debian11, new Date(2025, 0, 1)).map((p) => p.status)).toEqual(['done', 'current', 'upcoming'])
  })

  it('skips phases without a date', () => {
    expect(osPhases(support({ security_until: '2030-01-01' }), new Date(2026, 0, 1)).map((p) => p.key)).toEqual(['security'])
  })
})

describe('os counts', () => {
  const counts = { ended: 2, ending_soon: 1, unknown: 3, untracked: 1, supported: 5 }

  it('sums hosts at risk and all hosts', () => {
    expect(osAtRisk(counts)).toBe(3)
    expect(osTotal(counts)).toBe(12)
  })
})
