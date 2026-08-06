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

import { ref, computed } from 'vue'
import {
  fetchEdition,
  fetchLicenseStatus,
  type Edition,
  type EditionResponse,
  type LicenseStatus,
  type QuotaResource,
} from '@/services/editionApi'
import { sseBus } from '@/services/sseBus'
import { onProbePayload } from '@/services/authGuard'

/** Community < Personal < Pro. Absent from this map means "unknown to this build". */
const EDITION_RANK = { community: 0, personal: 1, pro: 2 } as const

const edition = ref<EditionResponse | null>(null)
const licenseStatus = ref<LicenseStatus | null>(null)
const loaded = ref(false)

async function load() {
  if (loaded.value) return
  try {
    edition.value = await fetchEdition()
  } catch {
    edition.value = { edition: 'community', organisation_name: '', features: {} }
  }
  loaded.value = true
}

async function reload() {
  loaded.value = false
  await load()
}

async function loadLicenseStatus() {
  try {
    licenseStatus.value = await fetchLicenseStatus()
  } catch {
    licenseStatus.value = null
  }
}

// SSE events that change the quota counters — auto-reload on any of them.
// Covers user-initiated actions AND label/annotation-driven auto-discovery.
const QUOTA_EVENTS = [
  'endpoint.discovered',
  'endpoint.removed',
  'heartbeat.created',
  'heartbeat.deleted',
  'certificate.created',
  'certificate.deleted',
  'status.component_changed',
  'agent.created',
  'agent.revoked',
  'agent.deleted',
] as const

let reloadTimer: ReturnType<typeof setTimeout> | null = null
function scheduleQuotaReload() {
  if (reloadTimer) clearTimeout(reloadTimer)
  reloadTimer = setTimeout(() => {
    reloadTimer = null
    void reload()
  }, 200)
}

for (const ev of QUOTA_EVENTS) {
  sseBus.on(ev, scheduleQuotaReload)
}

// The session probe already fetches /edition — reuse its answer rather than
// firing a second request for the same thing.
onProbePayload((payload) => {
  if (payload && typeof payload === 'object' && 'edition' in payload) {
    edition.value = payload as EditionResponse
  }
})

/**
 * An edition transition happens server-side and nothing pushes it. Refreshing
 * when the tab comes back to the foreground realigns what is offered without
 * asking the user to reconnect (FR-032), and costs one request per return.
 */
if (typeof document !== 'undefined') {
  let lastVisibilityReload = 0
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState !== 'visible') return
    if (Date.now() - lastVisibilityReload < 30_000) return
    lastVisibilityReload = Date.now()
    void reload()
  })
}

// Start loading immediately on first import
load()

export function useEdition() {
  const editionName = computed<Edition>(() => edition.value?.edition || 'community')

  /**
   * Rank in the Community < Personal < Pro order, or null when the engine
   * reports an edition this build does not know.
   */
  const editionRank = computed<number | null>(() => {
    const rank = EDITION_RANK[editionName.value as keyof typeof EDITION_RANK]
    return rank ?? null
  })

  const isPro = computed(() => editionName.value === 'pro')
  const isPersonal = computed(() => editionName.value === 'personal')

  /**
   * Community is now a positive test, not the absence of Pro. An unknown
   * edition is therefore treated as paid — the safe direction: it suppresses
   * upsell rather than nagging someone who has paid (D10, FR-045).
   */
  const isCommunity = computed(() => editionName.value === 'community')
  const isPaid = computed(() => !isCommunity.value)

  const organisationName = computed(() => edition.value?.organisation_name || '')
  const statusURL = computed(() => edition.value?.status_url || '')

  /**
   * The minimum edition that opens a capability, read from the backend
   * registry. Duplicating the matrix in TypeScript is what would let the two
   * drift apart, so an unknown capability returns null and callers fall back to
   * a generic message rather than guessing "pro".
   */
  function requiredEditionFor(feature: string): Edition | null {
    return edition.value?.feature_editions?.[feature] ?? null
  }

  /**
   * Whether the running edition permits a capability, regardless of whether it
   * is configured. `features.smtp` is the one flag that folds the two together
   * — it is `permitted && configured` — so telling "your edition does not allow
   * this" apart from "your edition allows it but it is not set up" needs this
   * (FR-015). Returns false when either side of the comparison is unknown.
   */
  function editionPermits(feature: string): boolean {
    const required = requiredEditionFor(feature)
    if (required === null || editionRank.value === null) return false
    const requiredRank = EDITION_RANK[required as keyof typeof EDITION_RANK]
    return requiredRank !== undefined && editionRank.value >= requiredRank
  }

  const licenseMessage = computed(() => licenseStatus.value?.message || '')
  const licenseStatusValue = computed(() => licenseStatus.value?.status || '')

  function hasFeature(name: string): boolean {
    return edition.value?.features[name] === true
  }

  function getQuota(resource: QuotaResource) {
    return computed(() => {
      const quota = edition.value?.quotas?.[resource]
      const used = quota?.used ?? 0
      const limit = quota?.limit ?? -1
      const isUnlimited = limit === -1
      const remaining = isUnlimited ? Infinity : Math.max(0, limit - used)
      const isAtLimit = !isUnlimited && used >= limit
      const nearLimit = !isUnlimited && limit > 0 && used / limit >= 0.8

      return {
        used,
        limit,
        remaining,
        isUnlimited,
        isAtLimit,
        nearLimit,
      }
    })
  }

  const personalization = computed(() => hasFeature('personalization'))

  return {
    edition,
    editionName,
    editionRank,
    isPro,
    isPersonal,
    isPaid,
    isCommunity,
    requiredEditionFor,
    editionPermits,
    organisationName,
    statusURL,
    hasFeature,
    personalization,
    load,
    reload,
    getQuota,
    licenseStatus,
    licenseMessage,
    licenseStatusValue,
    loadLicenseStatus,
  }
}
