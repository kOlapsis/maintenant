// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import UpdatesPage from '@/pages/UpdatesPage.vue'
import type { AgentOSSupport, OSSupportState } from '@/services/agentApi'
import type { ImageUpdate } from '@/services/updateApi'

const api = vi.hoisted(() => ({ updates: [] as unknown[] }))

vi.mock('@/services/updateApi', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/updateApi')>()
  return {
    ...actual,
    fetchUpdates: vi.fn(async () => ({ updates: api.updates, last_scan: '', next_scan: '' })),
    fetchUpdateSummary: vi.fn(async () => ({
      last_scan: '',
      next_scan: '',
      scan_status: 'idle',
      counts: { critical: 0, recommended: 1, available: 0, up_to_date: 0, pinned: 0 },
    })),
    fetchHostOS: vi.fn(async () => ({ hosts: [], eol_table: null })),
  }
})

vi.mock('@/services/agentApi', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/agentApi')>()
  return { ...actual, listAgents: vi.fn(async () => ({ agents: [] })) }
})

vi.mock('@/services/sseBus', async () => {
  const { ref } = await import('vue')
  return {
    sseBus: {
      on: vi.fn(),
      off: vi.fn(),
      connect: vi.fn(),
      disconnect: vi.fn(),
      connected: ref(true),
    },
  }
})

vi.mock('@/composables/useEdition', async () => {
  const { ref } = await import('vue')
  return { useEdition: () => ({ edition: ref(null), hasFeature: () => false }) }
})

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {}, hash: '' }),
  useRouter: () => ({ replace: vi.fn() }),
}))

function update(extra: Partial<ImageUpdate> = {}): ImageUpdate {
  return {
    id: 'upd-1',
    container_id: 'ctr-1',
    container_name: 'db',
    image: 'postgres:18.7-alpine3.24',
    current_tag: '18.7-alpine3.24',
    current_digest: '',
    latest_tag: '18.8-alpine3.24',
    latest_digest: '',
    update_type: 'minor',
    published_at: null,
    changelog_url: '',
    changelog_summary: '',
    has_breaking_changes: false,
    risk_score: 50,
    status: 'available',
    detected_at: '2026-10-13T10:00:00Z',
    ...extra,
  }
}

function support(state: OSSupportState, cycle: string | null, days: number | null): AgentOSSupport {
  return {
    state,
    product: 'postgresql',
    cycle,
    active_until: null,
    security_until: '2030-11-14',
    extended_until: null,
    days_remaining: days,
    table_source: 'embedded',
  }
}

async function mountRow(extra: Partial<ImageUpdate>, openGate = false) {
  api.updates = [update(extra)]
  const wrapper = mount(UpdatesPage, {
    global: {
      stubs: {
        teleport: true,
        FeatureGate: openGate ? { template: '<div><slot /></div>' } : true,
        FeatureHint: true,
        OsSupportTimeline: true,
      },
    },
  })
  await flushPromises()
  return wrapper
}

describe('UpdatesPage: major tag and support of an update row', () => {
  beforeEach(() => {
    localStorage.clear()
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('shows neither fact when the update carries none', async () => {
    const wrapper = await mountRow({})
    expect(wrapper.find('[data-testid="update-major"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="update-support"]').exists()).toBe(false)
  })

  it('shows the next major tag when present', async () => {
    const wrapper = await mountRow({ major_tag: '19.0-alpine3.24' })
    expect(wrapper.find('[data-testid="update-major"]').text()).toBe('Next major: 19.0-alpine3.24')
    expect(wrapper.find('[data-testid="update-support"]').exists()).toBe(false)
  })

  it('shortens a digest-like major tag as the target tag is', async () => {
    const wrapper = await mountRow({ major_tag: 'sha-0123456789abcdef0123' })
    expect(wrapper.find('[data-testid="update-major"]').text()).toBe('Next major: sha-01234567')
  })

  it('shows the support cycle without the major tag when only support is present', async () => {
    const wrapper = await mountRow({ support: support('supported', '18', 1455) })
    expect(wrapper.find('[data-testid="update-major"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="update-support"]').text()).toBe('PostgreSQL 18: 3 y 11 mo left')
  })

  it('shows both facts together', async () => {
    const wrapper = await mountRow({
      major_tag: '19.0-alpine3.24',
      support: support('supported', '18', 1455),
    })
    expect(wrapper.find('[data-testid="update-major"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="update-support"]').exists()).toBe(true)
  })

  it.each<[string, AgentOSSupport, string, string]>([
    ['supported', support('supported', '18', 1455), 'PostgreSQL 18: 3 y 11 mo left', 'text-mnt-status-ok'],
    ['security_only', support('security_only', '16', 400), 'PostgreSQL 16: 1 y 1 mo left', 'text-mnt-status-ok'],
    ['ending_soon', support('ending_soon', '14', 40), 'PostgreSQL 14: Ends in 40 days', 'text-mnt-status-warn'],
    ['ended', support('ended', '13', -200), 'PostgreSQL 13: Ended 6 mo ago', 'text-mnt-status-down'],
    ['ended without a date', support('ended', '13', null), 'PostgreSQL 13: Support ended', 'text-mnt-status-down'],
    ['no cycle', support('supported', null, 1455), '3 y 11 mo left', 'text-mnt-status-ok'],
  ])('words and colors the %s state', async (_name, value, text, color) => {
    const wrapper = await mountRow({ support: value })
    const el = wrapper.find('[data-testid="update-support"]')
    expect(el.text()).toBe(text)
    expect(el.classes()).toContain(color)
  })

  it('does not truncate the line that carries the facts', async () => {
    const wrapper = await mountRow({
      major_tag: '19.0-alpine3.24',
      support: support('supported', '18', 1455),
    })
    const line = wrapper.find('[data-testid="update-support"]').element.parentElement!
    expect(line.className).not.toContain('truncate')
    expect(line.className).toContain('flex-wrap')
  })

  it('breaks the next major tag on words, not on letters', async () => {
    const wrapper = await mountRow({ major_tag: '19.0-alpine3.24' })
    const classes = wrapper.find('[data-testid="update-major"]').classes()
    expect(classes).toContain('break-words')
    expect(classes).not.toContain('break-all')
  })
})

describe('UpdatesPage: row colors follow the urgency group', () => {
  beforeEach(() => {
    localStorage.clear()
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  function jump(wrapper: Awaited<ReturnType<typeof mountRow>>) {
    return wrapper.findAll('p').find((p) => p.text().includes('→') && p.classes().includes('font-bold'))!
  }

  it.each<[number, string]>([
    [30, 'text-mnt-green-400'],
    [31, 'text-mnt-status-warn'],
    [80, 'text-mnt-status-warn'],
    [81, 'text-mnt-status-down'],
  ])('colors the version jump of a major at score %i with %s', async (risk_score, color) => {
    const wrapper = await mountRow({ update_type: 'major', risk_score })
    expect(jump(wrapper).classes()).toContain(color)
  })

  it('does not show a major at score 20 in red', async () => {
    const wrapper = await mountRow({ update_type: 'major', risk_score: 20 }, true)
    const row = jump(wrapper).element.parentElement!.parentElement!
    expect(jump(wrapper).classes()).not.toContain('text-mnt-status-down')
    expect(row.innerHTML).not.toContain('status-down')
  })

  it.each([30, 31, 80, 81])('gives the version jump and the score the same color at score %i', async (risk_score) => {
    const wrapper = await mountRow({ update_type: 'major', risk_score }, true)
    const score = wrapper.findAll('span').find((s) => s.text() === String(risk_score))!
    const color = jump(wrapper).classes().find((c) => c.startsWith('text-mnt-') && c !== 'text-mnt-muted')
    expect(score.classes()).toContain(color)
  })

  it('hides the right-hand column below the sm breakpoint', async () => {
    const wrapper = await mountRow({})
    const column = jump(wrapper).element.parentElement!
    expect(column.className).toContain('hidden')
    expect(column.className).toContain('sm:block')
  })
})
