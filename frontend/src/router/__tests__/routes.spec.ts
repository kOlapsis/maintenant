// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from 'vitest'
import type { RouteRecordRaw } from 'vue-router'
import { buildRoutes } from '@/router/routes'

// 043-public-surfaces-hardening, US3 scenario 5, FR-022, FR-050.

const dashboardRoutes = [
  'dashboard', 'containers', 'endpoints', 'heartbeats', 'certificates', 'alerts', 'channels',
  'status-admin', 'webhooks', 'updates', 'security', 'services', 'tasks', 'workloads', 'pods',
  'cluster', 'nodes', 'escalation', 'agents', 'outbound-heartbeats', 'editions',
]

function names(routes: readonly RouteRecordRaw[]): string[] {
  return routes.flatMap((r) => [...(typeof r.name === 'string' ? [r.name] : []), ...names(r.children ?? [])])
}

function paths(routes: readonly RouteRecordRaw[], prefix = ''): string[] {
  return routes.flatMap((r) => {
    const full = r.path.startsWith('/') ? r.path : `${prefix.replace(/\/$/, '')}/${r.path}`
    return [full, ...paths(r.children ?? [], full)]
  })
}

describe('buildRoutes', () => {
  it('registers only the public status page in the status subdomain context (FR-022)', () => {
    const routes = buildRoutes(true)

    expect(routes.map((r) => r.path).sort()).toEqual(['/', '/status'])
    const registered = names(routes)
    for (const name of dashboardRoutes) {
      expect(registered, `${name} must not exist on the status subdomain`).not.toContain(name)
    }
    const registeredPaths = paths(routes)
    for (const path of ['/dashboard', '/containers', '/heartbeats', '/status-admin']) {
      expect(registeredPaths, `${path} must not exist on the status subdomain`).not.toContain(path)
    }
    const root = routes.find((r) => r.path === '/')
    expect(root?.children?.some((c) => c.name === 'status-public'), '/ leads to the public status page').toBe(true)
  })

  it('keeps the dashboard routes outside the status subdomain', () => {
    const registered = names(buildRoutes(false))
    for (const name of ['dashboard', 'containers', 'heartbeats', 'status-admin', 'status-public']) {
      expect(registered).toContain(name)
    }
    expect(paths(buildRoutes(false))).toContain('/status')
  })
})
