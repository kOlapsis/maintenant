import { ref, computed, type ComputedRef } from 'vue'
import { useContainersStore } from '@/stores/containers'
import { useEdition } from '@/composables/useEdition'

export type Tier = 0 | 1 | 2 | 3

const COOLDOWN_MS = 30 * 24 * 60 * 60 * 1000

export function tierFromCount(count: number): Tier {
  if (count < 10) return 0
  if (count <= 24) return 1
  if (count <= 49) return 2
  return 3
}

function storageKey(tier: Tier): string {
  return `mnt:banner:pro-tier-${tier}`
}

export interface ProBannerHandle {
  tier: ComputedRef<Tier | null>
  count: ComputedRef<number | null>
  visible: ComputedRef<boolean>
  dismiss: () => void
}

export function useProBanner(): ProBannerHandle {
  const containers = useContainersStore()
  const { isPro } = useEdition()

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
    dismissNonce.value
    const t = tier.value
    if (t === null || t === 0) return false
    const raw = localStorage.getItem(storageKey(t)) ?? localStorage.getItem(`pb:banner:pro-tier-${t}`)
    const ts = Number(raw)
    return Number.isFinite(ts) && Date.now() - ts < COOLDOWN_MS
  })

  const visible = computed(
    () => !isPro.value && tier.value !== null && tier.value !== 0 && !dismissed.value,
  )

  function dismiss(): void {
    const t = tier.value
    if (t === null || t === 0) return
    localStorage.setItem(storageKey(t), String(Date.now()))
    dismissNonce.value++
  }

  return { tier, count, visible, dismiss }
}