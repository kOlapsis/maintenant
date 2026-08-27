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
  type HistoryWindowSpec,
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

  /**
   * "inactive" means no license is configured at all, the normal state of a
   * Community instance, not a fault. Flagging it raised a red CRITICAL banner on
   * an installation that had simply never bought anything.
   *
   * The two update-window statuses deliberately stay out of this list: a build
   * running past its window is an anomaly, and the whole point is to say so.
   */
  const LICENSE_STATUS_NOT_A_FAULT = ['', 'active', 'inactive']

  const hasLicenseIssue = computed(
    () => !LICENSE_STATUS_NOT_A_FAULT.includes(licenseStatusValue.value),
  )

  /**
   * Classification lives here and nowhere else. It used to be duplicated in
   * DefaultLayout and EditionsPage, which is how a status ends up handled in one
   * of the two and falling through to a generic label in the other.
   */
  const licenseSeverity = computed<'warning' | 'critical'>(() => {
    const s = licenseStatusValue.value
    return s === 'grace' || s === 'unreachable' || s === 'update_window_grace'
      ? 'warning'
      : 'critical'
  })

  const licenseLabel = computed(() => {
    switch (licenseStatusValue.value) {
      case 'grace':
        return 'GRACE PERIOD'
      case 'unreachable':
        return 'LICENSE UNREACHABLE'
      case 'expired':
        return 'LICENSE EXPIRED'
      case 'canceled':
        return 'LICENSE CANCELED'
      case 'revoked':
        return 'LICENSE REVOKED'
      case 'unknown':
        return 'LICENSE INVALID'
      // The license itself is untouched here: it is perpetual. What ran out is
      // the right to run a version released after the window closed.
      case 'update_window_grace':
        return 'UPDATES EXPIRED'
      case 'update_window_ended':
        return 'EDITION SUSPENDED'
      default:
        return 'LICENSE'
    }
  })

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

  /**
   * The resource-history catalogue, exactly as the engine declares it. An
   * engine that does not report one gives an empty catalogue: the interface
   * shows what it knows and invents nothing, which is the whole point of
   * reading this from the server rather than from a table compiled in here.
   */
  const historyWindows = computed<HistoryWindowSpec[]>(
    () => edition.value?.resource_history?.windows ?? [],
  )

  /** The largest window the running edition opens, or '' when unknown. */
  const maxHistoryWindow = computed(() => edition.value?.resource_history?.max_window ?? '')

  const maxHistoryWindowSeconds = computed(
    () => edition.value?.resource_history?.max_window_seconds ?? 0,
  )

  function isWindowOpen(name: string): boolean {
    const spec = historyWindows.value.find((w) => w.window === name)
    if (!spec) return false
    return spec.seconds <= maxHistoryWindowSeconds.value
  }

  /** The edition that opens a window, or null when this build has no catalogue. */
  function requiredEditionForWindow(name: string): Edition | null {
    return historyWindows.value.find((w) => w.window === name)?.min_edition ?? null
  }

  /**
   * The fallback a view drops to when the window it was showing closes under
   * it. The catalogue is ordered by duration, so the last open entry is the
   * largest one.
   */
  const largestOpenWindow = computed<string>(() => {
    const open = historyWindows.value.filter((w) => w.seconds <= maxHistoryWindowSeconds.value)
    return open.length > 0 ? open[open.length - 1]!.window : ''
  })

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
    historyWindows,
    maxHistoryWindow,
    maxHistoryWindowSeconds,
    isWindowOpen,
    requiredEditionForWindow,
    largestOpenWindow,
    personalization,
    load,
    reload,
    getQuota,
    licenseStatus,
    licenseMessage,
    licenseStatusValue,
    hasLicenseIssue,
    licenseSeverity,
    licenseLabel,
    loadLicenseStatus,
  }
}
