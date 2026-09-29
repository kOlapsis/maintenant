// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0
import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { ref, computed } from 'vue'
import type { EditionResponse } from '@/services/editionApi'
import EditionsPage from '@/pages/EditionsPage.vue'

const edition = ref<EditionResponse | null>(null)

vi.mock('@/composables/useEdition', () => ({
  useEdition: () => ({
    edition,
    editionName: computed(() => 'community'),
    editionRank: computed(() => 0),
    licenseMessage: computed(() => ''),
    hasLicenseIssue: computed(() => false),
    licenseSeverity: computed(() => 'warning'),
    licenseLabel: computed(() => 'LICENSE'),
    loadLicenseStatus: () => Promise.resolve(),
  }),
}))

vi.mock('@/composables/useFeedbackUrl', () => ({
  useFeedbackUrl: () => ({ feedbackUrl: computed(() => '') }),
}))

function limitsRow(wrapper: ReturnType<typeof mount>, label: string): string[] {
  const row = wrapper.findAll('tr').find((tr) => tr.findAll('td')[0]?.text() === label)
  expect(row, label).toBeDefined()
  return row!
    .findAll('td')
    .slice(1)
    .map((td) => td.text())
}

describe('EditionsPage limits', () => {
  it('renders the caps the engine declares for every tier', () => {
    edition.value = {
      edition: 'community',
      organisation_name: '',
      features: {},
      tiers: {
        community: {
          endpoints: 10,
          heartbeats: 5,
          certificates: 5,
          status_components: 3,
          agent_hosts: 0,
        },
        personal: {
          endpoints: -1,
          heartbeats: -1,
          certificates: -1,
          status_components: -1,
          agent_hosts: 20,
        },
        pro: {
          endpoints: -1,
          heartbeats: -1,
          certificates: -1,
          status_components: -1,
          agent_hosts: -1,
        },
      },
    }
    const wrapper = mount(EditionsPage, { global: { stubs: ['EditionBadge', 'InlineAlert'] } })

    expect(limitsRow(wrapper, 'Endpoints')).toEqual(['10', 'Unlimited', 'Unlimited'])
    expect(limitsRow(wrapper, 'Heartbeats')).toEqual(['5', 'Unlimited', 'Unlimited'])
    expect(limitsRow(wrapper, 'Certificate monitors')).toEqual(['5', 'Unlimited', 'Unlimited'])
    expect(limitsRow(wrapper, 'Status page components')).toEqual(['3', 'Unlimited', 'Unlimited'])
    expect(limitsRow(wrapper, 'Machines monitored')).toEqual([
      'This machine only',
      '20',
      'Unlimited',
    ])
  })

  it('shows no limit row when the engine reports no tiers', () => {
    edition.value = { edition: 'community', organisation_name: '', features: {} }
    const wrapper = mount(EditionsPage, { global: { stubs: ['EditionBadge', 'InlineAlert'] } })

    expect(wrapper.findAll('td').some((td) => td.text() === 'Endpoints')).toBe(false)
  })
})
