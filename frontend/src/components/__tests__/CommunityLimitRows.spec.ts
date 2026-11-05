// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises, RouterLinkStub } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import EndpointRow from '@/components/EndpointRow.vue'
import EndpointCard from '@/components/EndpointCard.vue'
import EndpointDetail from '@/components/EndpointDetail.vue'
import CertificateRow from '@/components/CertificateRow.vue'
import CertificateDetail from '@/components/CertificateDetail.vue'
import { useEdition } from '@/composables/useEdition'
import { getEndpoint, listChecks } from '@/services/endpointApi'
import { getCertificate } from '@/services/certificateApi'
import {
  certMonitor,
  editionLinks,
  editionPayload,
  endpoint,
  LIMIT_TEXT,
} from './communityLimitFixtures'

// 051-community-label-limits, FR-006, FR-007, FR-013: the lists and the detail
// panels show an element left out by the cap, with a link to the Editions page,
// and say nothing on an element that is monitored.

vi.mock('@/services/editionApi', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/services/editionApi')>()),
  fetchEdition: vi.fn(() => new Promise(() => {})),
  fetchLicenseStatus: vi.fn(async () => null),
}))
vi.mock('@/services/uptimeApi', () => ({
  fetchEndpointDailyUptime: vi.fn(async () => []),
}))
vi.mock('@/services/endpointApi', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/services/endpointApi')>()),
  getEndpoint: vi.fn(),
  listChecks: vi.fn(),
  listEndpoints: vi.fn(async () => ({ endpoints: [], total: 0 })),
  deleteEndpoint: vi.fn(),
  checkEndpointNow: vi.fn(),
}))
vi.mock('@/services/certificateApi', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/services/certificateApi')>()),
  getCertificate: vi.fn(),
  listCertificates: vi.fn(async () => ({ certificates: [], total: 0 })),
  checkCertificateNow: vi.fn(),
}))
vi.mock('@/composables/useConfirm', () => ({ useConfirm: () => async () => true }))

const global = { stubs: { RouterLink: RouterLinkStub, EndpointEventTimeline: true, CertificateChecksHistory: true } }

beforeEach(() => {
  setActivePinia(createPinia())
  useEdition().edition.value = editionPayload('community')
})

const listViews = {
  EndpointRow: (limited: boolean) => mount(EndpointRow, { props: { endpoint: endpoint({ limited }) }, global }),
  EndpointCard: (limited: boolean) => mount(EndpointCard, { props: { endpoint: endpoint({ limited }) }, global }),
  CertificateRow: (limited: boolean) =>
    mount(CertificateRow, { props: { certificate: certMonitor({ limited }) }, global }),
}

describe('lists: an element left out by the Community limit', () => {
  for (const [name, view] of Object.entries(listViews)) {
    it(`${name} shows the state and a link to the Editions page`, async () => {
      const wrapper = view(true)
      await flushPromises()
      expect(wrapper.text()).toMatch(LIMIT_TEXT)
      expect(editionLinks(wrapper)).toBeGreaterThan(0)
    })

    // FR-013, SC-002: under the cap nothing changes.
    it(`${name} says nothing of the limit on a monitored element`, async () => {
      const wrapper = view(false)
      await flushPromises()
      expect(wrapper.text()).not.toMatch(/Not monitored/i)
      expect(editionLinks(wrapper)).toBe(0)
    })
  }
})

describe('detail panels: an element left out by the Community limit', () => {
  it('EndpointDetail shows the state and a link to the Editions page', async () => {
    vi.mocked(getEndpoint).mockResolvedValue({ endpoint: endpoint({ limited: true }) })
    vi.mocked(listChecks).mockResolvedValue({ endpoint_id: 'ep-shop', checks: [], total: 0, has_more: false })
    const wrapper = mount(EndpointDetail, { props: { endpointId: 'ep-shop' }, global })
    await flushPromises()

    expect(wrapper.text()).toMatch(LIMIT_TEXT)
    expect(editionLinks(wrapper)).toBeGreaterThan(0)
  })

  it('CertificateDetail shows the state and a link to the Editions page', async () => {
    vi.mocked(getCertificate).mockResolvedValue({ certificate: certMonitor({ limited: true }) })
    const wrapper = mount(CertificateDetail, { props: { certificateId: 'cert-shop' }, global })
    await flushPromises()

    expect(wrapper.text()).toMatch(LIMIT_TEXT)
    expect(editionLinks(wrapper)).toBeGreaterThan(0)
  })

  it('EndpointDetail says nothing of the limit on a monitored endpoint', async () => {
    vi.mocked(getEndpoint).mockResolvedValue({ endpoint: endpoint({ limited: false }) })
    vi.mocked(listChecks).mockResolvedValue({ endpoint_id: 'ep-shop', checks: [], total: 0, has_more: false })
    const wrapper = mount(EndpointDetail, { props: { endpointId: 'ep-shop' }, global })
    await flushPromises()

    expect(wrapper.text()).not.toMatch(/Not monitored/i)
  })
})
