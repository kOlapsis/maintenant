// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import type { RuntimeStatus } from '@/services/runtimeApi'

const fetchRuntimeStatus = vi.hoisted(() => vi.fn())

vi.mock('@/services/runtimeApi', () => ({ fetchRuntimeStatus }))
vi.mock('@/services/containerApi', () => ({ listContainers: vi.fn() }))
vi.mock('@/composables/useToast', () => ({ showToast: vi.fn() }))
vi.mock('@/services/sseBus', () => ({
  sseBus: { on: vi.fn(), off: vi.fn(), connect: vi.fn(), disconnect: vi.fn(), connected: false, suspended: false },
}))

import { useContainersStore } from '../containers'
import { useRuntimeStore } from '../runtime'

function status(context: RuntimeStatus['context'], runtime: RuntimeStatus['runtime']): RuntimeStatus {
  return { context, runtime, connected: true, label: '', detected_at: '2026-09-30T00:00:00Z', metadata: {} }
}

beforeEach(() => {
  setActivePinia(createPinia())
  fetchRuntimeStatus.mockReset()
})

describe('containers store: the runtime it describes is the one the server reports', () => {
  it('knows a Swarm manager from the status read at load, with no event needed', async () => {
    fetchRuntimeStatus.mockResolvedValue(status('swarm', 'docker'))
    const containers = useContainersStore()

    await useRuntimeStore().fetchStatus()

    expect(containers.isSwarmMode).toBe(true)
    expect(containers.isKubernetesMode).toBe(false)
    expect(containers.runtimeLabel).toBe('Docker')
  })

  it('knows a Kubernetes runtime', async () => {
    fetchRuntimeStatus.mockResolvedValue(status('kubernetes', 'kubernetes'))
    const containers = useContainersStore()

    await useRuntimeStore().fetchStatus()

    expect(containers.isKubernetesMode).toBe(true)
    expect(containers.runtimeName).toBe('kubernetes')
    expect(containers.runtimeLabel).toBe('Kubernetes')
  })
})
