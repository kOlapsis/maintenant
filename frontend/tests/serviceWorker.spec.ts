// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

// Acceptance tests of 047-audit-small-fixes, C13: the service worker never caches an API answer.

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
  // A urlPattern this parser cannot read (string, function) would slip past every assertion below.
  expect(routes).toHaveLength(block.match(/urlPattern\s*:/g)!.length)
  return routes
}

function handlerFor(path: string): string | undefined {
  const href = new URL(path, 'http://localhost:8080').href
  return runtimeRoutes().find((r) => r.pattern.test(href))?.handler
}

describe('service worker runtime caching', () => {
  // FR-007, US3 scenario 1, SC-005: no runtimeCaching rule targets /api/, sensitive families or not.
  it.each([
    '/api/v1/containers/0123abcd/logs',
    '/api/v1/containers/0123abcd/logs?lines=200',
    '/api/v1/status/subscribers',
    '/api/v1/escalation-policies',
    '/api/v1/escalation-policies/p1',
    '/api/v1/escalation-runs/r1',
    '/api/v1/agents',
    '/api/v1/agents/enrollment-tokens',
    '/api/v1/security/insights',
    '/api/v1/security/posture/containers',
    '/api/v1/cve',
    '/api/v1/cve/0123abcd',
    '/api/v1/containers',
    '/api/v1/endpoints',
    '/api/v1/alerts',
    '/api/v1/dashboard/summary',
    '/api/v1/edition',
    '/api/v1/edition?__probe=1',
    '/api/v1/containers/events',
    '/api/v1/channels',
    '/api/v1/status/smtp',
    '/api/v1/webhooks',
  ])('leaves %s to the network, untouched by any rule', (path) => {
    expect(handlerFor(path)).toBeUndefined()
  })

  // FR-007, US3 scenario 1: static files keep CacheFirst.
  it.each([
    '/assets/index-abc123.js',
    '/assets/index-abc123.css',
    '/assets/inter-latin.woff2',
    '/icons/icon-192.png',
    '/logo.svg',
    '/favicon.ico',
  ])('keeps %s on CacheFirst', (path) => {
    expect(handlerFor(path)).toBe('CacheFirst')
  })

  // FR-007: only static files stay cached, so no cache of the config may be named api-*.
  it('declares no api-* cache', () => {
    expect(source).not.toMatch(/cacheName:\s*[`'"]api-/)
  })
})
