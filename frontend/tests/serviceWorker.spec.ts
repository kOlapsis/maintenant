// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

// Acceptance tests of 047-audit-small-fixes, C13: the service worker never serves sensitive answers from a cache.

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, it, expect } from 'vitest'

const source = readFileSync(resolve(__dirname, '..', 'vite.config.ts'), 'utf8')

type Route = { pattern: RegExp; handler: string }

/** The runtimeCaching routes of vite.config.ts, in registration order: Workbox serves a URL with the first one that matches. */
function runtimeRoutes(): Route[] {
  const start = source.indexOf('runtimeCaching')
  expect(start).toBeGreaterThan(-1)
  const block = source.slice(start)
  const entry = /urlPattern:\s*\/((?:\\\/|[^/\n])+)\/([a-z]*)\s*,\s*handler:\s*'([A-Za-z]+)'/g
  const routes: Route[] = []
  for (const m of block.matchAll(entry)) {
    routes.push({ pattern: new RegExp(m[1]!, m[2]), handler: m[3]! })
  }
  expect(routes.length).toBeGreaterThan(0)
  return routes
}

function handlerFor(path: string): string | undefined {
  const href = new URL(path, 'http://localhost:8080').href
  return runtimeRoutes().find((r) => r.pattern.test(href))?.handler
}

describe('service worker runtime caching', () => {
  // FR-007, US3 scenario 1: logs, subscribers, escalation, agents, security and CVE are NetworkOnly.
  it.each([
    '/api/v1/containers/0123abcd/logs',
    '/api/v1/containers/0123abcd/logs?lines=200',
    '/api/v1/status/subscribers',
    '/api/v1/escalation-policies',
    '/api/v1/escalation-policies/p1',
    '/api/v1/escalation-policies/p1/runs',
    '/api/v1/agents',
    '/api/v1/agents/a1',
    '/api/v1/agents/enrollment-tokens',
    '/api/v1/security/insights',
    '/api/v1/security/summary',
    '/api/v1/security/posture/containers',
    '/api/v1/security/acknowledgments',
    '/api/v1/cve',
    '/api/v1/cve/0123abcd',
  ])('serves %s from the network only', (path) => {
    expect(handlerFor(path)).toBe('NetworkOnly')
  })

  // FR-007, US3 scenario 1: the other API routes keep their strategy.
  it.each([
    ['/api/v1/containers', 'NetworkFirst'],
    ['/api/v1/endpoints', 'NetworkFirst'],
    ['/api/v1/alerts', 'NetworkFirst'],
    ['/api/v1/dashboard/summary', 'NetworkFirst'],
    ['/api/v1/edition?__probe=1', 'NetworkOnly'],
    ['/api/v1/containers/events', undefined],
    ['/api/v1/channels', undefined],
    ['/assets/index-abc123.js', 'CacheFirst'],
  ])('keeps %s on %s', (path, handler) => {
    expect(handlerFor(path)).toBe(handler)
  })
})
