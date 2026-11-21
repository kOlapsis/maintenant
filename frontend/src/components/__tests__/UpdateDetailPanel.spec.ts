// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import UpdateDetailPanel from '@/components/UpdateDetailPanel.vue'
import type { AgentOSSupport } from '@/services/agentApi'
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

describe('UpdateDetailPanel — major tag and support', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  const supported: AgentOSSupport = {
    state: 'supported',
    product: 'postgresql',
    cycle: '18',
    active_until: null,
    security_until: '2030-11-14',
    extended_until: null,
    days_remaining: 1455,
    table_source: 'embedded',
  }

  it('shows neither fact when the detail carries none', async () => {
    const wrapper = mountPanel({ ...baseDetail })
    await new Promise((r) => setTimeout(r, 50))

    expect(wrapper.find('[data-testid="update-major"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="update-support"]').exists()).toBe(false)
  })

  it('shows the next major tag when present', async () => {
    const wrapper = mountPanel({ ...baseDetail, major_tag: '19.0-alpine3.24' })
    await new Promise((r) => setTimeout(r, 50))

    expect(wrapper.find('[data-testid="update-major"]').text()).toBe('Next major: 19.0-alpine3.24')
    expect(wrapper.find('[data-testid="update-support"]').exists()).toBe(false)
  })

  it('shows the support cycle with the color of its state', async () => {
    const wrapper = mountPanel({ ...baseDetail, support: supported })
    await new Promise((r) => setTimeout(r, 50))

    const el = wrapper.find('[data-testid="update-support"]')
    expect(el.text()).toBe('PostgreSQL 18: 3 y 11 mo left')
    expect(el.classes()).toContain('text-mnt-status-ok')
    expect(wrapper.find('[data-testid="update-major"]').exists()).toBe(false)
  })

  it('words an ended cycle and colors it as down', async () => {
    const wrapper = mountPanel({
      ...baseDetail,
      support: { ...supported, state: 'ended', cycle: '13', days_remaining: -200 },
    })
    await new Promise((r) => setTimeout(r, 50))

    const el = wrapper.find('[data-testid="update-support"]')
    expect(el.text()).toBe('PostgreSQL 13: Ended 6 mo ago')
    expect(el.classes()).toContain('text-mnt-status-down')
  })

  it('words an ending cycle and colors it as a warning', async () => {
    const wrapper = mountPanel({
      ...baseDetail,
      support: { ...supported, state: 'ending_soon', cycle: '14', days_remaining: 40 },
    })
    await new Promise((r) => setTimeout(r, 50))

    const el = wrapper.find('[data-testid="update-support"]')
    expect(el.text()).toBe('PostgreSQL 14: Ends in 40 days')
    expect(el.classes()).toContain('text-mnt-status-warn')
  })
})

describe('UpdateDetailPanel — update type badge', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it.each(['major', 'minor', 'patch', 'digest_only'])('uses the neutral style for %s', async (update_type) => {
    const wrapper = mountPanel({ ...baseDetail, update_type })
    await new Promise((r) => setTimeout(r, 50))

    const badge = wrapper.findAll('span').find((s) => s.text() === update_type)!
    expect(badge.classes()).toEqual(
      expect.arrayContaining(['bg-mnt-sev-neutral-solid/10', 'text-mnt-muted']),
    )
    expect(badge.classes()).not.toContain('text-mnt-status-down')
    expect(badge.classes()).not.toContain('text-mnt-status-warn')
    expect(badge.classes()).not.toContain('text-mnt-green-400')
  })
})

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
