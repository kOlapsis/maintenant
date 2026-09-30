// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, beforeEach, vi } from 'vitest'
import { shallowMount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import { nextTick, ref } from 'vue'

const handlers = vi.hoisted(() => new Map<string, (e: MessageEvent) => void>())

vi.mock('@/services/sseBus', () => ({
  sseBus: {
    on: (type: string, fn: (e: MessageEvent) => void) => handlers.set(type, fn),
    off: (type: string) => handlers.delete(type),
    connect: vi.fn(),
    disconnect: vi.fn(),
    connected: false,
    suspended: false,
  },
}))
vi.mock('@/services/runtimeApi', () => ({ fetchRuntimeStatus: vi.fn() }))
vi.mock('@/services/containerApi', () => ({ listContainers: vi.fn() }))
vi.mock('@/composables/useToast', () => ({ showToast: vi.fn() }))
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))
vi.mock('@/composables/useVisibleInterval', () => ({ useVisibleInterval: vi.fn() }))
vi.mock('@/composables/useTheme', () => ({ useTheme: () => ({ theme: ref('system'), setTheme: vi.fn() }) }))
vi.mock('@/composables/useFeedbackUrl', () => ({ useFeedbackUrl: () => ({ feedbackUrl: ref('') }) }))
vi.mock('@/stores/dashboard', () => ({
  useDashboardStore: () => ({
    fetchAll: vi.fn(),
    connectAllSSE: vi.fn(),
    disconnectAllSSE: vi.fn(),
    refetchForFilter: vi.fn(),
    searchQuery: '',
    globalStats: { running: 0, warnings: 0, incidents: 0 },
  }),
}))
vi.mock('@/stores/alerts', () => ({
  useAlertsStore: () => ({ activeAlerts: { critical: [], warning: [], info: [] }, totalActiveCount: 0 }),
}))
vi.mock('@/stores/resources', () => ({
  useResourcesStore: () => ({ summary: null, selected: null, fetchSummary: vi.fn(), reconcile: vi.fn() }),
}))
vi.mock('@/stores/storage', () => ({ useStorageStore: () => ({ connected: true }) }))
vi.mock('@/stores/agents', () => ({ useAgentsStore: () => ({ agents: [], fetchAgents: vi.fn() }) }))

import AppHeader from '@/components/AppHeader.vue'
import { useRuntimeStore } from '@/stores/runtime'

function emit(type: string, data: unknown) {
  handlers.get(type)?.(new MessageEvent(type, { data: JSON.stringify(data) }))
}

beforeEach(() => {
  setActivePinia(createPinia())
  handlers.clear()
})

describe('AppHeader runtime banner', () => {
  it('shows RUNTIME OFFLINE while the server reports its container runtime lost', async () => {
    const runtime = useRuntimeStore()
    runtime.startListening()
    const wrapper = shallowMount(AppHeader)
    const banner = () => wrapper.find('[label="RUNTIME OFFLINE"]')
    expect(banner().exists()).toBe(false)

    emit('runtime.availability_changed', { name: 'docker', connected: false })
    await nextTick()
    expect(banner().exists()).toBe(true)

    emit('runtime.availability_changed', { name: 'docker', connected: true })
    await nextTick()
    expect(banner().exists()).toBe(false)
    runtime.stopListening()
  })
})
