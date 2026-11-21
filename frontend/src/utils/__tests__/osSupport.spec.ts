// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from 'vitest'
import type { AgentOSSupport, OSSupportState, OSUnavailableReason } from '@/services/agentApi'
import type { HostOS, ImageSupport } from '@/services/updateApi'
import {
  groupHostsByOS,
  groupImagesByProduct,
  osAtRisk,
  osCanonicalName,
  osCycleText,
  osPhases,
  osProductName,
  osTimeLeft,
  osTotal,
} from '../osSupport'

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

describe('product names', () => {
  it.each([
    ['postgresql', 'PostgreSQL'],
    ['mysql', 'MySQL'],
    ['mariadb', 'MariaDB'],
    ['redis', 'Redis'],
    ['mongodb', 'MongoDB'],
    ['nginx', 'nginx'],
    ['debian', 'Debian'],
    ['unlisted', 'unlisted'],
  ])('names %s as %s', (slug, name) => {
    expect(osProductName(slug)).toBe(name)
  })

  it('names a cycle with its product', () => {
    expect(osCanonicalName(support({ product: 'mongodb', cycle: '8.0' }))).toBe('MongoDB 8.0')
  })

  it.each([
    [support({ product: 'postgresql', cycle: '18', days_remaining: 1455 }), 'PostgreSQL 18: 3 y 11 mo left'],
    [support({ product: 'nginx', cycle: '1.28', state: 'ending_soon', days_remaining: 40 }), 'nginx 1.28: Ends in 40 days'],
    [support({ product: 'redis', cycle: null, days_remaining: 1455 }), '3 y 11 mo left'],
    [support({ product: null, cycle: '18', days_remaining: 1455 }), '3 y 11 mo left'],
    [support({ product: 'mysql', cycle: '8.0', state: 'ended' }), 'MySQL 8.0: Support ended'],
  ])('words the cycle text of %#', (value, text) => {
    expect(osCycleText(value)).toBe(text)
  })
})

function image(
  id: string,
  name: string,
  overrides: Partial<AgentOSSupport>,
  agentId?: string,
): ImageSupport {
  return {
    container_id: id,
    container_name: name,
    image: `${name}:tag`,
    support: support(overrides),
    ...(agentId ? { agent_id: agentId } : {}),
  }
}

describe('groupImagesByProduct', () => {
  const images = [
    image('c1', 'web', { state: 'supported', product: 'nginx', cycle: '1.28', days_remaining: 900 }),
    image('c2', 'db-b', { state: 'ended', product: 'postgresql', cycle: '13', days_remaining: -200 }),
    image('c3', 'cache', { state: 'ending_soon', product: 'redis', cycle: '7.2', days_remaining: 30 }),
    image('c4', 'db-a', { state: 'ended', product: 'postgresql', cycle: '13', days_remaining: -200 }, 'agent-1'),
    image('c5', 'db-new', { state: 'supported', product: 'postgresql', cycle: '18', days_remaining: 1455 }),
    image('c6', 'old-db', { state: 'ended', product: 'mysql', cycle: '5.7', days_remaining: -1000 }),
  ]

  it('groups containers by product and cycle, containers sorted by name', () => {
    const group = groupImagesByProduct(images).find((g) => g.name === 'PostgreSQL 13')
    expect(group?.containers.map((c) => c.container_id)).toEqual(['c4', 'c2'])
  })

  it('orders ended oldest first, then ending soon, then supported', () => {
    expect(groupImagesByProduct(images).map((g) => g.name)).toEqual([
      'MySQL 5.7',
      'PostgreSQL 13',
      'Redis 7.2',
      'nginx 1.28',
      'PostgreSQL 18',
    ])
  })

  it('returns nothing for no image', () => {
    expect(groupImagesByProduct([])).toEqual([])
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
