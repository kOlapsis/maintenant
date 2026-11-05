// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { computed, defineComponent, h } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import DefaultLayout from '@/layouts/DefaultLayout.vue'
import { useEdition } from '@/composables/useEdition'
import { editionPayload } from '@/components/__tests__/communityLimitFixtures'

// 051-community-label-limits, US4 scenario 1, FR-016, SC-006: the Community
// banner driven by the container count is gone, whatever the count; the licence
// banner stays.

const fleet = vi.hoisted(() => {
  window.matchMedia ??= ((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  })) as typeof window.matchMedia
  return { count: 0 }
})

vi.mock('@/services/editionApi', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/services/editionApi')>()),
  fetchEdition: vi.fn(() => new Promise(() => {})),
  fetchLicenseStatus: vi.fn(() => new Promise(() => {})),
}))
vi.mock('@/stores/containers', () => ({
  useContainersStore: () => ({
    get containerCount() {
      return fleet.count
    },
    get groups() {
      return [
        {
          name: 'stack',
          source: 'docker',
          containers: Array.from({ length: fleet.count }, (_, i) => ({ id: String(i), archived: false })),
        },
      ]
    },
    activeContainers: [],
  }),
}))
vi.mock('@/stores/swarm', () => ({ useSwarmStore: () => ({ loadInfo: vi.fn() }) }))
vi.mock('@/stores/runtime', () => ({ useRuntimeStore: () => ({ fetchStatus: vi.fn(), startListening: vi.fn() }) }))
vi.mock('@/stores/storage', () => ({ useStorageStore: () => ({ fetchStatus: vi.fn(), startListening: vi.fn() }) }))
vi.mock('@/stores/resources', () => ({ useResourcesStore: () => ({ selected: null, entityQuery: undefined }) }))
vi.mock('@/composables/useFleetRuntimes', () => ({
  useFleetRuntimes: () => ({ availableRuntimes: computed(() => ['docker']) }),
}))
vi.mock('@/composables/useAppVersion', async () => {
  const { ref } = await import('vue')
  return { useAppVersion: () => ({ version: ref('v1.6.0') }) }
})

const Blank = defineComponent({ render: () => h('div') })

async function mountLayout() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: Blank },
      { path: '/dashboard', name: 'dashboard', component: Blank },
      { path: '/editions', name: 'editions', component: Blank },
    ],
  })
  await router.push('/dashboard')
  await router.isReady()
  const wrapper = mount(DefaultLayout, {
    global: {
      plugins: [router],
      stubs: {
        AppHeader: true,
        DetailSlideOver: true,
        ToastContainer: true,
        HostFilterDropdown: true,
        DemoModeBanner: true,
        SuspendedChannelsBanner: true,
        ConfirmDialog: true,
        RouterView: true,
      },
    },
  })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  localStorage.clear()
  useEdition().licenseStatus.value = null
})

describe('DefaultLayout on Community', () => {
  for (const count of [15, 30, 88]) {
    it(`shows no edition banner with ${count} containers`, async () => {
      fleet.count = count
      useEdition().edition.value = editionPayload('community', {
        endpoints: { used: 7, limit: 10, unmonitored: 0 },
      })
      const text = (await mountLayout()).text()

      expect(text).not.toContain('EDITIONS')
      expect(text).not.toContain('Hitting the Community limits')
      expect(text).not.toContain('Personal removes every limit')
      expect(text).not.toContain('containers in production')
      expect(text).not.toMatch(/€149|€29/)
    })
  }

  // FR-016: the licence state banner does not change.
  it('still shows the licence banner', async () => {
    fleet.count = 88
    useEdition().edition.value = editionPayload('community')
    useEdition().licenseStatus.value = {
      status: 'grace',
      plan: 'personal',
      message: 'Your licence could not be verified, renew it within 7 days',
      verified_at: '',
      expires_at: '',
      updates_until: '',
      update_grace_until: '',
    }
    const text = (await mountLayout()).text()

    expect(text).toContain('Your licence could not be verified')
    expect(text).toContain('GRACE PERIOD')
  })
})
