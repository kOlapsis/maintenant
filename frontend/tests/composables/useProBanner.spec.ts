import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { createApp, defineComponent, nextTick, reactive, ref } from 'vue'
import { tierFromCount, useProBanner, type ProBannerHandle } from '@/composables/useProBanner'
import { useContainersStore as _useContainersStore } from '@/stores/containers'
import { useEdition as _useEdition } from '@/composables/useEdition'

vi.mock('@/stores/containers', () => ({ useContainersStore: vi.fn() }))
vi.mock('@/composables/useEdition', () => ({ useEdition: vi.fn() }))

const mockedUseContainersStore = vi.mocked(_useContainersStore)
const mockedUseEdition = vi.mocked(_useEdition)

type ContainerMock = { containerCount: number; groups: unknown[] }

function mountProBanner(
  store: ContainerMock,
  isPro = ref(false),
): { handle: ProBannerHandle; unmount: () => void } {
  mockedUseContainersStore.mockReturnValue(store as ReturnType<typeof _useContainersStore>)
  mockedUseEdition.mockReturnValue(
    { isPro } as ReturnType<typeof _useEdition>,
  )

  let handle!: ProBannerHandle
  const app = createApp(
    defineComponent({
      setup() { handle = useProBanner(); return {} },
      render() { return null },
    }),
  )
  app.mount(document.createElement('div'))
  return { handle, unmount: () => app.unmount() }
}

function groups(n: number): unknown[] {
  return [{ name: 'g', source: 'docker', containers: Array.from({ length: n }, (_, i) => ({ id: String(i), archived: false })) }]
}

// ─── Phase 2 checkpoint ───────────────────────────────────────────────────────

describe('tierFromCount', () => {
  it('returns 0 for 0', () => expect(tierFromCount(0)).toBe(0))
  it('returns 0 for 9', () => expect(tierFromCount(9)).toBe(0))
  it('returns 1 for 10', () => expect(tierFromCount(10)).toBe(1))
  it('returns 1 for 24', () => expect(tierFromCount(24)).toBe(1))
  it('returns 2 for 25', () => expect(tierFromCount(25)).toBe(2))
  it('returns 2 for 49', () => expect(tierFromCount(49)).toBe(2))
  it('returns 3 for 50', () => expect(tierFromCount(50)).toBe(3))
  it('returns 3 for 100', () => expect(tierFromCount(100)).toBe(3))
})

// ─── US1 — tier 1 & 2 visibility ─────────────────────────────────────────────

describe('useProBanner — tier 1 & 2 visibility', () => {
  it('tier 1 at count=15', async () => {
    const { handle, unmount } = mountProBanner({ containerCount: 15, groups: groups(15) })
    await nextTick()
    expect(handle.tier.value).toBe(1)
    expect(handle.count.value).toBe(15)
    expect(handle.visible.value).toBe(true)
    unmount()
  })

  it('tier 2 at count=30', async () => {
    const { handle, unmount } = mountProBanner({ containerCount: 30, groups: groups(30) })
    await nextTick()
    expect(handle.tier.value).toBe(2)
    expect(handle.count.value).toBe(30)
    expect(handle.visible.value).toBe(true)
    unmount()
  })

  it('tier 2 at count=27', async () => {
    const { handle, unmount } = mountProBanner({ containerCount: 27, groups: groups(27) })
    await nextTick()
    expect(handle.tier.value).toBe(2)
    expect(handle.count.value).toBe(27)
    expect(handle.visible.value).toBe(true)
    unmount()
  })

  it('tier 1 inclusive upper bound at count=24', async () => {
    const { handle, unmount } = mountProBanner({ containerCount: 24, groups: groups(24) })
    await nextTick()
    expect(handle.tier.value).toBe(1)
    unmount()
  })

  it('tier 2 inclusive lower bound at count=25', async () => {
    const { handle, unmount } = mountProBanner({ containerCount: 25, groups: groups(25) })
    await nextTick()
    expect(handle.tier.value).toBe(2)
    unmount()
  })

  it('reactive — tier becomes visible when store hydrates after mount', async () => {
    const store = reactive<ContainerMock>({ containerCount: 0, groups: [] })
    const { handle, unmount } = mountProBanner(store)
    await nextTick()
    expect(handle.tier.value).toBeNull()
    expect(handle.count.value).toBeNull()
    expect(handle.visible.value).toBe(false)
    // Store hydrates asynchronously (e.g. after layout mount, fetchContainers resolves).
    store.containerCount = 48
    store.groups = groups(48)
    await nextTick()
    expect(handle.tier.value).toBe(2)
    expect(handle.count.value).toBe(48)
    expect(handle.visible.value).toBe(true)
    unmount()
  })

  it('reactive — tier updates when container count grows past a tier boundary', async () => {
    const store = reactive<ContainerMock>({ containerCount: 15, groups: groups(15) })
    const { handle, unmount } = mountProBanner(store)
    await nextTick()
    expect(handle.tier.value).toBe(1)
    store.containerCount = 30
    store.groups = groups(30)
    await nextTick()
    expect(handle.tier.value).toBe(2)
    expect(handle.count.value).toBe(30)
    unmount()
  })
})

// ─── US2 — tier 3 visibility ──────────────────────────────────────────────────

describe('useProBanner — tier 3 visibility', () => {
  it('tier 3 at count=50', async () => {
    const { handle, unmount } = mountProBanner({ containerCount: 50, groups: groups(50) })
    await nextTick()
    expect(handle.tier.value).toBe(3)
    expect(handle.count.value).toBe(50)
    expect(handle.visible.value).toBe(true)
    unmount()
  })

  it('tier 3 at count=80', async () => {
    const { handle, unmount } = mountProBanner({ containerCount: 80, groups: groups(80) })
    await nextTick()
    expect(handle.tier.value).toBe(3)
    expect(handle.count.value).toBe(80)
    unmount()
  })

  it('tier 3 at count=1000 (no upper bound)', async () => {
    const { handle, unmount } = mountProBanner({ containerCount: 1000, groups: groups(1) })
    await nextTick()
    expect(handle.tier.value).toBe(3)
    unmount()
  })

  it('tier 2 at count=49 (upper bound tier 2, not tier 3)', async () => {
    const { handle, unmount } = mountProBanner({ containerCount: 49, groups: groups(49) })
    await nextTick()
    expect(handle.tier.value).toBe(2)
    unmount()
  })
})

// ─── US3 — tier 0 & guard ─────────────────────────────────────────────────────

describe('useProBanner — tier 0 & guard', () => {
  it('tier 0 when count=0 with non-empty groups (legitimate empty)', async () => {
    const { handle, unmount } = mountProBanner({ containerCount: 0, groups: [{ containers: [] }] })
    await nextTick()
    expect(handle.tier.value).toBe(0)
    expect(handle.visible.value).toBe(false)
    unmount()
  })

  it('tier 0 at count=5', async () => {
    const { handle, unmount } = mountProBanner({ containerCount: 5, groups: groups(5) })
    await nextTick()
    expect(handle.tier.value).toBe(0)
    expect(handle.visible.value).toBe(false)
    unmount()
  })

  it('tier 0 at count=9', async () => {
    const { handle, unmount } = mountProBanner({ containerCount: 9, groups: groups(9) })
    await nextTick()
    expect(handle.tier.value).toBe(0)
    expect(handle.visible.value).toBe(false)
    unmount()
  })

  it('FR-015 guard — null tier when count=0 and groups empty (store not loaded)', async () => {
    const { handle, unmount } = mountProBanner({ containerCount: 0, groups: [] })
    await nextTick()
    expect(handle.tier.value).toBeNull()
    expect(handle.visible.value).toBe(false)
    unmount()
  })

  it('tier 1 at count=10 (transition from tier 0)', async () => {
    const { handle, unmount } = mountProBanner({ containerCount: 10, groups: groups(10) })
    await nextTick()
    expect(handle.tier.value).toBe(1)
    expect(handle.visible.value).toBe(true)
    unmount()
  })

  it('dismiss no-op when tier=0: no localStorage write', async () => {
    const { handle, unmount } = mountProBanner({ containerCount: 5, groups: groups(5) })
    await nextTick()
    handle.dismiss()
    expect(localStorage.getItem('pb:banner:pro-tier-0')).toBeNull()
    unmount()
  })

  it('dismiss no-op when tier=null: no localStorage write', async () => {
    const { handle, unmount } = mountProBanner({ containerCount: 0, groups: [] })
    await nextTick()
    handle.dismiss()
    expect(localStorage.getItem('pb:banner:pro-tier-0')).toBeNull()
    unmount()
  })
})

// ─── US4 — tier-keyed dismissal & cooldown ───────────────────────────────────

describe('useProBanner — tier-keyed dismissal & cooldown', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-15T12:00:00Z'))
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('FR-010 dismiss writes tier key and hides banner immediately', async () => {
    const { handle, unmount } = mountProBanner({ containerCount: 15, groups: groups(15) })
    await nextTick()
    expect(handle.visible.value).toBe(true)
    handle.dismiss()
    const ts = Number(localStorage.getItem('pb:banner:pro-tier-1'))
    expect(Number.isFinite(ts)).toBe(true)
    expect(ts).toBe(Date.now())
    expect(handle.visible.value).toBe(false)
    unmount()
  })

  it('FR-011 cooldown active — banner hidden when dismissed 1s ago', async () => {
    localStorage.setItem('pb:banner:pro-tier-1', String(Date.now() - 1000))
    const { handle, unmount } = mountProBanner({ containerCount: 15, groups: groups(15) })
    await nextTick()
    expect(handle.visible.value).toBe(false)
    unmount()
  })

  it('FR-011 cooldown expired — banner shown after 31 days', async () => {
    localStorage.setItem('pb:banner:pro-tier-1', String(Date.now() - 31 * 24 * 60 * 60 * 1000))
    const { handle, unmount } = mountProBanner({ containerCount: 15, groups: groups(15) })
    await nextTick()
    expect(handle.visible.value).toBe(true)
    unmount()
  })

  it('FR-012 tier-keying — tier-1 dismissal does not block tier-2', async () => {
    localStorage.setItem('pb:banner:pro-tier-1', String(Date.now() - 1000))
    const { handle, unmount } = mountProBanner({ containerCount: 27, groups: groups(27) })
    await nextTick()
    expect(handle.tier.value).toBe(2)
    expect(handle.visible.value).toBe(true)
    unmount()
  })

  it('FR-016 legacy key pb:banner:support-prompt is ignored', async () => {
    localStorage.setItem('pb:banner:support-prompt', String(Date.now()))
    const { handle, unmount } = mountProBanner({ containerCount: 15, groups: groups(15) })
    await nextTick()
    expect(handle.visible.value).toBe(true)
    expect(localStorage.getItem('pb:banner:support-prompt')).not.toBeNull()
    unmount()
  })

  it('corrupted localStorage value treated as non-dismissed', async () => {
    localStorage.setItem('pb:banner:pro-tier-1', 'not-a-number')
    const { handle, unmount } = mountProBanner({ containerCount: 15, groups: groups(15) })
    await nextTick()
    expect(handle.visible.value).toBe(true)
    unmount()
  })
})

// ─── US5 — license gate ───────────────────────────────────────────────────────

describe('useProBanner — license gate', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('FR-013 pro license hides banner (tier still computed)', async () => {
    const { handle, unmount } = mountProBanner({ containerCount: 30, groups: groups(30) }, ref(true))
    await nextTick()
    expect(handle.tier.value).toBe(2)
    expect(handle.count.value).toBe(30)
    expect(handle.visible.value).toBe(false)
    unmount()
  })

  it('community license shows banner', async () => {
    const { handle, unmount } = mountProBanner({ containerCount: 30, groups: groups(30) }, ref(false))
    await nextTick()
    expect(handle.visible.value).toBe(true)
    unmount()
  })

  it('reactive to isPro change: false → visible becomes true', async () => {
    const isPro = ref(true)
    const { handle, unmount } = mountProBanner({ containerCount: 30, groups: groups(30) }, isPro)
    await nextTick()
    expect(handle.visible.value).toBe(false)
    isPro.value = false
    await nextTick()
    expect(handle.visible.value).toBe(true)
    unmount()
  })

  it('dismissal preserved when license expires (cooldown active)', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-15T12:00:00Z'))
    localStorage.setItem('pb:banner:pro-tier-2', String(Date.now() - 1000))
    const isPro = ref(true)
    const { handle, unmount } = mountProBanner({ containerCount: 30, groups: groups(30) }, isPro)
    await nextTick()
    expect(handle.visible.value).toBe(false) // Pro masque
    isPro.value = false
    await nextTick()
    expect(handle.visible.value).toBe(false) // cooldown still active
    unmount()
    vi.useRealTimers()
  })

  it('no dismissal: false → true when pro expires', async () => {
    const isPro = ref(true)
    const { handle, unmount } = mountProBanner({ containerCount: 30, groups: groups(30) }, isPro)
    await nextTick()
    expect(handle.visible.value).toBe(false)
    isPro.value = false
    await nextTick()
    expect(handle.visible.value).toBe(true)
    unmount()
  })
})
