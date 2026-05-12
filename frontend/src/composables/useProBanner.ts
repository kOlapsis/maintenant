import { ref, computed, onMounted, type Ref, type ComputedRef } from 'vue'
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
  return `pb:banner:pro-tier-${tier}`
}

export interface ProBannerHandle {
  tier: Readonly<Ref<Tier | null>>
  count: Readonly<Ref<number | null>>
  visible: ComputedRef<boolean>
  dismiss: () => void
}

export function useProBanner(): ProBannerHandle {
  const containers = useContainersStore()
  const { isEnterprise } = useEdition()

  const tier = ref<Tier | null>(null)
  const count = ref<number | null>(null)
  const dismissed = ref(false)

  onMounted(() => {
    const c = containers.containerCount
    const groups = containers.groups

    if (c === 0 && groups.length === 0) return

    tier.value = tierFromCount(c)
    count.value = c

    if (tier.value !== 0) {
      const raw = localStorage.getItem(storageKey(tier.value))
      const ts = Number(raw)
      dismissed.value = Number.isFinite(ts) && Date.now() - ts < COOLDOWN_MS
    }
  })

  const visible = computed(
    () => !isEnterprise.value && tier.value !== null && tier.value !== 0 && !dismissed.value,
  )

  function dismiss(): void {
    if (tier.value === null || tier.value === 0) return
    dismissed.value = true
    localStorage.setItem(storageKey(tier.value), String(Date.now()))
  }

  return { tier, count, visible, dismiss }
}
