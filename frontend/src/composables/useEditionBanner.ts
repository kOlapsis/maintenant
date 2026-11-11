import { ref, computed, type ComputedRef } from 'vue'
import { useContainersStore } from '@/stores/containers'
import { useEdition } from '@/composables/useEdition'

export type Tier = 0 | 1 | 2 | 3

const COOLDOWN_MS = 7 * 24 * 60 * 60 * 1000

export function tierFromCount(count: number): Tier {
  if (count < 10) return 0
  if (count <= 24) return 1
  if (count <= 49) return 2
  return 3
}

function storageKey(tier: Tier): string {
  return `mnt:banner:edition-tier-${tier}`
}

const LIMIT_KEY = 'mnt:banner:edition-limit'

function dismissedRecently(keys: string[]): boolean {
  let raw: string | null = null
  for (const key of keys) {
    raw = localStorage.getItem(key)
    if (raw !== null) break
  }
  const ts = Number(raw)
  return Number.isFinite(ts) && Date.now() - ts < COOLDOWN_MS
}

/**
 * Keys this banner used before. A dismissal is a user decision, so a rename
 * must not resurrect a banner someone already sent away — the older `pb:`
 * prefix was migrated the same way.
 */
function legacyStorageKeys(tier: Tier): string[] {
  return [`mnt:banner:pro-tier-${tier}`, `pb:banner:pro-tier-${tier}`]
}

export interface EditionBannerHandle {
  tier: ComputedRef<Tier | null>
  count: ComputedRef<number | null>
  visible: ComputedRef<boolean>
  dismiss: () => void
  unmonitoredEndpoints: ComputedRef<number>
  unmonitoredCertificates: ComputedRef<number>
  limitVisible: ComputedRef<boolean>
  dismissLimit: () => void
}

export function useEditionBanner(): EditionBannerHandle {
  const containers = useContainersStore()
  const { isCommunity, getQuota } = useEdition()
  const endpointQuota = getQuota('endpoints')
  const certificateQuota = getQuota('certificates')

  // localStorage isn't reactive — bump this to force dismissed recompute after dismiss().
  const dismissNonce = ref(0)

  const tier = computed<Tier | null>(() => {
    const c = containers.containerCount
    if (c === 0 && containers.groups.length === 0) return null
    return tierFromCount(c)
  })

  const count = computed<number | null>(() =>
    tier.value === null ? null : containers.containerCount,
  )

  const dismissed = computed<boolean>(() => {
    void dismissNonce.value
    const t = tier.value
    if (t === null || t === 0) return false
    return (
      dismissedRecently([storageKey(t), ...legacyStorageKeys(t)]) || dismissedRecently([LIMIT_KEY])
    )
  })

  const unmonitoredEndpoints = computed(() => endpointQuota.value.unmonitored)
  const unmonitoredCertificates = computed(() => certificateQuota.value.unmonitored)

  const limitDismissed = computed<boolean>(() => {
    void dismissNonce.value
    return dismissedRecently([LIMIT_KEY])
  })

  const limitVisible = computed(
    () =>
      isCommunity.value &&
      unmonitoredEndpoints.value + unmonitoredCertificates.value > 0 &&
      !limitDismissed.value,
  )

  const visible = computed(
    // Community only — not "anything that is not Pro". Personal is a paid
    // edition and must never be canvassed, and an edition this build does not
    // know is treated as paid too rather than nagged (D10, FR-040).
    () => isCommunity.value && tier.value !== null && tier.value !== 0 && !dismissed.value,
  )

  function dismiss(): void {
    const t = tier.value
    if (t === null || t === 0) return
    localStorage.setItem(storageKey(t), String(Date.now()))
    dismissNonce.value++
  }

  function dismissLimit(): void {
    localStorage.setItem(LIMIT_KEY, String(Date.now()))
    dismissNonce.value++
  }

  return {
    tier,
    count,
    visible,
    dismiss,
    unmonitoredEndpoints,
    unmonitoredCertificates,
    limitVisible,
    dismissLimit,
  }
}