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
import { fetchEdition, fetchLicenseStatus, type EditionResponse, type LicenseStatus, type QuotaResource } from '@/services/editionApi'

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

// Start loading immediately on first import
load()

export function useEdition() {
  const isEnterprise = computed(() => edition.value?.edition === 'enterprise')
  const isCommunity = computed(() => !isEnterprise.value)
  const organisationName = computed(() => edition.value?.organisation_name || '')

  const licenseMessage = computed(() => licenseStatus.value?.message || '')
  const licenseStatusValue = computed(() => licenseStatus.value?.status || '')

  function hasFeature(name: string): boolean {
    return edition.value?.features[name] === true
  }

  function getQuota(resource: QuotaResource) {
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
  }

  return {
    edition,
    isEnterprise,
    isCommunity,
    organisationName,
    hasFeature,
    load,
    reload,
    getQuota,
    licenseStatus,
    licenseMessage,
    licenseStatusValue,
    loadLicenseStatus,
  }
}
