// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import UpdatesPage from '@/pages/UpdatesPage.vue'
import AgentBadge from '@/components/AgentBadge.vue'
import { useAgentsStore } from '@/stores/agents'
import type { Agent, AgentOSSupport, OSSupportState } from '@/services/agentApi'
import type { ImageSupport } from '@/services/updateApi'

const api = vi.hoisted(() => ({ images: [] as unknown[], fail: false }))

vi.mock('@/services/updateApi', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/updateApi')>()
  return {
    ...actual,
    fetchUpdates: vi.fn(async () => ({ updates: [], last_scan: '', next_scan: '' })),
    fetchUpdateSummary: vi.fn(async () => ({
      last_scan: '',
      next_scan: '',
      scan_status: 'idle',
      counts: { critical: 0, recommended: 0, available: 0, up_to_date: 0, pinned: 0 },
    })),
    fetchHostOS: vi.fn(async () => ({ hosts: [], eol_table: null })),
    fetchImages: vi.fn(async () => {
      if (api.fail) throw new Error('HTTP 500')
      return { images: api.images }
    }),
  }
})

vi.mock('@/services/agentApi', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/agentApi')>()
  return { ...actual, listAgents: vi.fn(async () => ({ agents: [] })) }
})

vi.mock('@/services/sseBus', async () => {
  const { ref } = await import('vue')
  return {
    sseBus: { on: vi.fn(), off: vi.fn(), connect: vi.fn(), disconnect: vi.fn(), connected: ref(true) },
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

function support(state: OSSupportState, product: string, cycle: string, days: number | null): AgentOSSupport {
  return {
    state,
    product,
    cycle,
    active_until: '2027-01-01',
    security_until: '2030-11-14',
    extended_until: null,
    days_remaining: days,
    table_source: 'embedded',
  }
}

function image(
  id: string,
  name: string,
  value: AgentOSSupport,
  host: Partial<ImageSupport> = {},
): ImageSupport {
  return { container_id: id, container_name: name, image: `${name}:tag`, support: value, ...host }
}

async function mountPage(images: ImageSupport[], agents: Agent[] = []) {
  api.images = images
  useAgentsStore().agents = agents
  const wrapper = mount(UpdatesPage, {
    global: {
      stubs: { teleport: true, FeatureGate: true, FeatureHint: true, OsSupportTimeline: true },
    },
  })
  await flushPromises()
  return wrapper
}

function block(wrapper: Awaited<ReturnType<typeof mountPage>>) {
  return wrapper.find('section#software')
}

function rows(wrapper: Awaited<ReturnType<typeof mountPage>>) {
  return block(wrapper).findAll(':scope > ul > li')
}

describe('UpdatesPage: software lifecycle block', () => {
  beforeEach(() => {
    localStorage.clear()
    setActivePinia(createPinia())
    vi.clearAllMocks()
    api.images = []
    api.fail = false
  })

  const images = [
    image('c1', 'web', support('supported', 'nginx', '1.28', 900)),
    image('c2', 'db-b', support('ended', 'postgresql', '13', -200)),
    image('c3', 'cache', support('ending_soon', 'redis', '7.2', 40)),
    image('c4', 'db-a', support('ended', 'postgresql', '13', -200)),
    image('c5', 'db-new', support('security_only', 'postgresql', '18', 1455)),
    image('c6', 'docs', support('supported', 'mongodb', '8.0', 700)),
  ]

  it('is hidden when no image is tracked', async () => {
    const wrapper = await mountPage([])
    expect(block(wrapper).exists()).toBe(false)
  })

  it('stays hidden when the request fails', async () => {
    api.fail = true
    const wrapper = await mountPage(images)
    expect(block(wrapper).exists()).toBe(false)
  })

  it('counts containers in the header', async () => {
    const wrapper = await mountPage(images)
    expect(block(wrapper).find('h2').text()).toBe('Software lifecycle')
    expect(block(wrapper).find('h2 + span').text()).toBe('6')
  })

  it('groups containers by product and cycle, most urgent first', async () => {
    const wrapper = await mountPage(images)
    expect(rows(wrapper).map((r) => r.find('p.font-bold').text())).toEqual([
      'PostgreSQL 13',
      'Redis 7.2',
      'PostgreSQL 18',
      'MongoDB 8.0',
      'nginx 1.28',
    ])
  })

  it('lists the container names of a group', async () => {
    const wrapper = await mountPage(images)
    const chips = rows(wrapper)[0]!.findAll('ul > li').map((c) => c.text())
    expect(chips).toEqual(['db-a', 'db-b'])
  })

  it.each<[string, AgentOSSupport, string, string]>([
    ['supported', support('supported', 'nginx', '1.28', 900), '2 y 5 mo left', 'text-mnt-status-ok'],
    ['security_only', support('security_only', 'mysql', '8.4', 400), '1 y 1 mo left', 'text-mnt-status-ok'],
    ['ending_soon', support('ending_soon', 'redis', '7.2', 40), 'Ends in 40 days', 'text-mnt-status-warn'],
    ['ended', support('ended', 'mariadb', '10.5', -200), 'Ended 6 mo ago', 'text-mnt-status-down'],
  ])('words and colors the %s state', async (_state, value, text, color) => {
    const wrapper = await mountPage([image('c1', 'x', value)])
    const left = rows(wrapper)[0]!.find('p.font-black')
    expect(left.text()).toBe(text)
    expect(left.classes()).toContain(color)
  })

  it('shows the host of each container only when remote agents exist', async () => {
    const withAgents = await mountPage(
      [
        image('c1', 'db', support('ended', 'postgresql', '13', -200), {
          agent_id: 'agent-nas',
          agent_hostname: 'nas-02',
          agent_label: 'NAS cave',
        }),
        image('c2', 'db2', support('ended', 'postgresql', '13', -200)),
      ],
      [{ agent_id: 'agent-nas', hostname: 'nas-02', label: 'NAS cave', status: 'active' } as Agent],
    )
    const badges = withAgents
      .findAllComponents(AgentBadge)
      .map((b) => b.text().trim())
      .filter(Boolean)
      .sort()
    expect(badges).toEqual(['Local', 'NAS cave'])

    const without = await mountPage([image('c1', 'db', support('ended', 'postgresql', '13', -200))])
    expect(without.findAllComponents(AgentBadge)).toHaveLength(0)
  })

  it('does not scroll sideways: rows collapse to two columns and chips wrap', async () => {
    const wrapper = await mountPage(images)
    const row = rows(wrapper)[0]!
    expect(row.classes()).toContain('grid-cols-[minmax(0,1fr)_auto]')
    expect(row.find('ul').classes()).toEqual(expect.arrayContaining(['flex-wrap', 'min-w-0']))
    expect(block(wrapper).classes()).toContain('overflow-hidden')
  })
})
