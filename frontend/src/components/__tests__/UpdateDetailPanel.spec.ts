// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import UpdateDetailPanel from '@/components/UpdateDetailPanel.vue'
import type { ContainerUpdateDetail } from '@/services/updateApi'

// Mock the API module so tests don't hit the network
vi.mock('@/services/updateApi', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/updateApi')>()
  return {
    ...actual,
    fetchContainerUpdate: vi.fn(),
    fetchContainerCves: vi.fn(),
  }
})

// Mock the edition composable — Community edition for all tests
vi.mock('@/composables/useEdition', () => ({
  useEdition: () => ({
    hasFeature: () => false,
    edition: 'community',
  }),
}))

// Mock sseBus to prevent real SSE connections
vi.mock('@/services/sseBus', () => ({
  sseBus: { on: vi.fn(), off: vi.fn() },
}))

import { fetchContainerUpdate } from '@/services/updateApi'

const baseDetail: ContainerUpdateDetail = {
  container_id: 'ctr1',
  container_name: 'nginx',
  image: 'nginx:1.24.0',
  current_tag: '1.24.0',
  current_digest: 'sha256:abc',
  latest_tag: '1.26.0',
  latest_digest: 'sha256:def',
  update_type: 'minor',
  risk_score: 0,
  active_cves: [],
  pinned: false,
  source_url: '',
  previous_digest: '',
  update_command: 'docker pull nginx:1.26.0',
  rollback_command: '',
  changelog_url: '',
  changelog_summary: '',
  has_breaking_changes: false,
}

function mountPanel(detail: ContainerUpdateDetail) {
  vi.mocked(fetchContainerUpdate).mockResolvedValue(detail)
  return mount(UpdateDetailPanel, {
    props: { containerId: 'ctr1' },
    global: {
      plugins: [createPinia()],
      stubs: {
        // Stub Pro-gated components to simplify test output
        FeatureGate: { template: '<div />' },
        RiskScoreGauge: { template: '<div />' },
        CveList: { template: '<div />' },
        ChangelogViewer: { template: '<div />' },
      },
    },
  })
}

describe('UpdateDetailPanel — tag filter section', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('hides tag filter section when neither tag_include nor tag_exclude is set', async () => {
    const wrapper = mountPanel({ ...baseDetail })
    await vi.waitUntil(() => !wrapper.find('[data-loading]').exists(), { timeout: 500 }).catch(
      () => {},
    )
    // Wait for async loading to complete
    await new Promise((r) => setTimeout(r, 50))
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).not.toContain('Tag Filter')
  })

  it('shows tag filter section with include pattern when tag_include is set', async () => {
    const wrapper = mountPanel({ ...baseDetail, tag_include: '^1\\.25' })
    await new Promise((r) => setTimeout(r, 50))
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('Tag Filter')
    expect(wrapper.text()).toContain('^1\\.25')
    expect(wrapper.text()).toContain('Include')
  })

  it('shows tag filter section with exclude pattern when tag_exclude is set', async () => {
    const wrapper = mountPanel({ ...baseDetail, tag_exclude: '(rc|beta|alpha)' })
    await new Promise((r) => setTimeout(r, 50))
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('Tag Filter')
    expect(wrapper.text()).toContain('(rc|beta|alpha)')
    expect(wrapper.text()).toContain('Exclude')
  })

  it('shows both include and exclude patterns when both are set', async () => {
    const wrapper = mountPanel({
      ...baseDetail,
      tag_include: '^20\\.',
      tag_exclude: '(rc|beta)',
    })
    await new Promise((r) => setTimeout(r, 50))
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('Tag Filter')
    expect(wrapper.text()).toContain('^20\\.')
    expect(wrapper.text()).toContain('(rc|beta)')
    expect(wrapper.text()).toContain('Include')
    expect(wrapper.text()).toContain('Exclude')
  })

  it('shows only the include row when only tag_include is set (no exclude row)', async () => {
    const wrapper = mountPanel({ ...baseDetail, tag_include: '^1\\.25' })
    await new Promise((r) => setTimeout(r, 50))
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('Include')
    expect(wrapper.text()).not.toContain('Exclude')
  })

  it('shows only the exclude row when only tag_exclude is set (no include row)', async () => {
    const wrapper = mountPanel({ ...baseDetail, tag_exclude: 'rc' })
    await new Promise((r) => setTimeout(r, 50))
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('Exclude')
    expect(wrapper.text()).not.toContain('Include')
  })
})
