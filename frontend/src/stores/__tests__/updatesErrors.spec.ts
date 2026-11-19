import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useUpdatesStore } from '@/stores/updates'

const api = vi.hoisted(() => ({
  pinVersion: vi.fn(),
  unpinVersion: vi.fn(),
  fetchUpdateSummary: vi.fn(),
  fetchContainerCves: vi.fn(),
  fetchRiskScores: vi.fn(),
  fetchExclusions: vi.fn(),
  addExclusion: vi.fn(),
  removeExclusion: vi.fn(),
}))

vi.mock('@/services/updateApi', () => api)
vi.mock('@/services/sseBus', () => ({ sseBus: { on: vi.fn(), off: vi.fn(), connect: vi.fn(), disconnect: vi.fn() } }))
vi.mock('@/composables/useEdition', () => ({ useEdition: () => ({ hasFeature: () => true }) }))

describe('updates store failures', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    for (const fn of Object.values(api)) fn.mockReset()
  })

  it.each([
    ['pinVersion', (s: ReturnType<typeof useUpdatesStore>) => s.pinVersion('c1', 'why'), 'pinVersion'],
    ['unpinVersion', (s: ReturnType<typeof useUpdatesStore>) => s.unpinVersion('c1'), 'unpinVersion'],
    ['fetchContainerCves', (s: ReturnType<typeof useUpdatesStore>) => s.fetchContainerCves('c1'), 'fetchContainerCves'],
    ['fetchRiskScores', (s: ReturnType<typeof useUpdatesStore>) => s.fetchRiskScores(), 'fetchRiskScores'],
    ['fetchExclusions', (s: ReturnType<typeof useUpdatesStore>) => s.fetchExclusions(), 'fetchExclusions'],
    ['addExclusion', (s: ReturnType<typeof useUpdatesStore>) => s.addExclusion('x', 'exact'), 'addExclusion'],
    ['removeExclusion', (s: ReturnType<typeof useUpdatesStore>) => s.removeExclusion('e1'), 'removeExclusion'],
  ] as const)('%s rejects with the API error', async (_name, run, apiName) => {
    api[apiName].mockRejectedValue(new Error('HTTP 500'))
    await expect(run(useUpdatesStore())).rejects.toThrow('HTTP 500')
  })
})
