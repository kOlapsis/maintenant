// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { RouterLinkStub, type VueWrapper } from '@vue/test-utils'
import type { ComponentPublicInstance } from 'vue'
import type { Edition, EditionResponse, QuotaEntry, QuotaResource } from '@/services/editionApi'
import type { Endpoint } from '@/services/endpointApi'
import type { CertMonitor } from '@/services/certificateApi'

// Shared fixtures of the 051-community-label-limits acceptance tests.

export const LIMIT_TEXT = /Not monitored:\s*Community limit/

const RANK: Record<string, number> = { community: 0, personal: 1, pro: 2 }

const WINDOWS = [
  { window: '1h', seconds: 3600, min_edition: 'community' },
  { window: '6h', seconds: 21600, min_edition: 'community' },
  { window: '24h', seconds: 86400, min_edition: 'community' },
  { window: '7d', seconds: 604800, min_edition: 'community' },
  { window: '30d', seconds: 2592000, min_edition: 'personal' },
  { window: '90d', seconds: 7776000, min_edition: 'pro' },
]

const MAX_WINDOW: Record<string, string> = { community: '7d', personal: '30d', pro: '90d' }

const FEATURE_EDITIONS: Record<string, Edition> = {
  alert_routing: 'community',
  smtp: 'personal',
  telegram: 'personal',
  multihost: 'personal',
  incidents: 'personal',
  slack: 'pro',
  teams: 'pro',
  alert_escalation: 'pro',
}

/** What GET /api/v1/edition reports for an edition, as the engine builds it. */
export function editionPayload(
  edition: 'community' | 'personal' | 'pro',
  quotas: Partial<Record<QuotaResource, QuotaEntry>> = {},
): EditionResponse {
  const features: Record<string, boolean> = {}
  for (const [f, min] of Object.entries(FEATURE_EDITIONS)) {
    features[f] = RANK[edition]! >= RANK[min]!
  }
  const max = WINDOWS.find((w) => w.window === MAX_WINDOW[edition])!
  return {
    edition,
    organisation_name: '',
    features,
    feature_editions: { ...FEATURE_EDITIONS },
    quotas,
    tiers: {
      community: { endpoints: 10, heartbeats: 5, certificates: 5, status_components: 3, agent_hosts: 0 },
      personal: { endpoints: -1, heartbeats: -1, certificates: -1, status_components: -1, agent_hosts: 20 },
      pro: { endpoints: -1, heartbeats: -1, certificates: -1, status_components: -1, agent_hosts: -1 },
    },
    resource_history: { max_window: max.window, max_window_seconds: max.seconds, windows: WINDOWS },
    suspended_channels: { count: 0, channels: [] },
  }
}

export function isEditionsTarget(to: unknown): boolean {
  if (typeof to === 'string') return to === '/editions'
  if (to && typeof to === 'object') {
    const o = to as { name?: unknown; path?: unknown }
    return o.name === 'editions' || o.path === '/editions'
  }
  return false
}

/** How many links of the wrapper lead to the Editions page, router links and plain anchors alike. */
export function editionLinks(wrapper: VueWrapper<ComponentPublicInstance>): number {
  const routed = wrapper.findAllComponents(RouterLinkStub).filter((l) => isEditionsTarget(l.props('to')))
  const anchors = wrapper.findAll('a').filter((a) => a.attributes('href') === '/editions')
  return routed.length + anchors.length
}

export function endpoint(over: Partial<Endpoint> = {}): Endpoint {
  return {
    id: 'ep-shop',
    container_name: 'shop',
    external_id: 'ext-shop',
    endpoint_type: 'http',
    target: 'https://shop.example.com',
    label_key: 'maintenant.endpoint.http',
    status: 'unknown',
    alert_state: 'normal',
    consecutive_failures: 0,
    consecutive_successes: 0,
    config: { interval: '30s', timeout: '10s', failure_threshold: 3, recovery_threshold: 2, tls_verify: true },
    active: true,
    first_seen_at: '2026-11-05T10:00:00Z',
    last_seen_at: '2026-11-05T10:00:00Z',
    source: 'label',
    agent_id: '00000000-0000-0000-0000-000000000000',
    limited: false,
    ...over,
  }
}

export function certMonitor(over: Partial<CertMonitor> = {}): CertMonitor {
  return {
    id: 'cert-shop',
    hostname: 'shop.example.com',
    port: 443,
    source: 'standalone',
    status: 'unknown',
    check_interval_seconds: 43200,
    warning_thresholds: [30, 14, 7, 3, 1],
    created_at: '2026-11-05T10:00:00Z',
    agent_id: '00000000-0000-0000-0000-000000000000',
    limited: false,
    ...over,
  }
}
