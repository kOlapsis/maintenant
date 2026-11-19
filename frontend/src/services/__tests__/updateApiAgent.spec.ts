// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { beforeEach, describe, expect, it, vi } from 'vitest'
import { apiFetch, apiFetchVoid, LOCAL_AGENT } from '@/services/apiFetch'
import { fetchContainerCves, fetchContainerUpdate, pinVersion, unpinVersion } from '@/services/updateApi'

vi.mock('@/services/apiFetch', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/services/apiFetch')>()),
  apiFetch: vi.fn(),
  apiFetchVoid: vi.fn(),
}))

const workload = 'shop/Deployment/api'

function urls(): string[] {
  return [...vi.mocked(apiFetch).mock.calls, ...vi.mocked(apiFetchVoid).mock.calls].map(([url]) => String(url))
}

describe('updateApi: the agent of a remote container travels with it', () => {
  beforeEach(() => {
    vi.mocked(apiFetch).mockReset().mockResolvedValue({})
    vi.mocked(apiFetchVoid).mockReset().mockResolvedValue(undefined)
  })

  it('names the remote agent on detail, CVEs, pin and unpin', async () => {
    await fetchContainerUpdate(workload, 'agent-prod')
    await fetchContainerCves(workload, 'agent-prod')
    await pinVersion(workload, 'freeze', 'agent-prod')
    await unpinVersion(workload, 'agent-prod')

    expect(urls()).toEqual([
      `/api/v1/updates/container/${workload}?agent_id=agent-prod`,
      `/api/v1/cve/${workload}?agent_id=agent-prod`,
      `/api/v1/updates/pin/${workload}?agent_id=agent-prod`,
      `/api/v1/updates/pin/${workload}?agent_id=agent-prod`,
    ])
  })

  it('leaves the local runtime implicit, as before', async () => {
    await fetchContainerUpdate('ctr-1')
    await pinVersion('ctr-1', 'freeze', LOCAL_AGENT)
    await unpinVersion('ctr-1', null)

    expect(urls()).toEqual([
      '/api/v1/updates/container/ctr-1',
      '/api/v1/updates/pin/ctr-1',
      '/api/v1/updates/pin/ctr-1',
    ])
  })
})
