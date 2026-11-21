// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import OsSupportRow, { type OsSupportRowItem } from '@/components/OsSupportRow.vue'
import AgentBadge from '@/components/AgentBadge.vue'
import type { AgentOSSupport, OSSupportState } from '@/services/agentApi'

function support(state: OSSupportState, days: number | null = 1455): AgentOSSupport {
  return {
    state,
    product: 'postgresql',
    cycle: '18',
    active_until: null,
    security_until: '2030-11-14',
    extended_until: null,
    days_remaining: days,
    table_source: 'embedded',
  }
}

function items(count: number): OsSupportRowItem[] {
  return Array.from({ length: count }, (_, i) => ({ key: `i${i}`, label: `item-${i}`, title: `title-${i}` }))
}

function mountRow(props: Record<string, unknown>) {
  return mount(OsSupportRow, {
    props: { state: 'supported', name: 'PostgreSQL 18', support: support('supported'), items: [], ...props },
    global: { stubs: { OsSupportTimeline: { template: '<div data-testid="timeline" />' } } },
  })
}

describe('OsSupportRow', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it.each<[OSSupportState, string, string]>([
    ['supported', '3 y 11 mo left', 'text-mnt-status-ok'],
    ['security_only', '3 y 11 mo left', 'text-mnt-status-ok'],
    ['ending_soon', 'Ends in 40 days', 'text-mnt-status-warn'],
    ['ended', 'Ended 6 mo ago', 'text-mnt-status-down'],
  ])('words and colors the time left of a %s row', (state, text, color) => {
    const days = { supported: 1455, security_only: 1455, ending_soon: 40, ended: -200 }[state as 'supported']
    const wrapper = mountRow({ state, support: support(state, days) })
    const left = wrapper.findAll('p').find((p) => p.text() === text)
    expect(left?.classes()).toContain(color)
    expect(wrapper.find('[data-testid="timeline"]').exists()).toBe(true)
  })

  it('shows the name with its tooltip', () => {
    const wrapper = mountRow({ title: 'Debian GNU/Linux 12' })
    const name = wrapper.find('p.font-bold')
    expect(name.text()).toBe('PostgreSQL 18')
    expect(name.attributes('title')).toBe('Debian GNU/Linux 12')
  })

  it('shows hints instead of the time and the bar for an unknown state', () => {
    const wrapper = mountRow({ state: 'unknown', name: '2 hosts without OS', hints: ['Update the agent', 'Mount it'] })
    expect(wrapper.text()).toContain('Update the agent · Mount it')
    expect(wrapper.find('[data-testid="timeline"]').exists()).toBe(false)
    expect(wrapper.find('p.font-black').exists()).toBe(false)
  })

  it('shows a note instead of the time and the bar for an untracked state', () => {
    const wrapper = mountRow({ state: 'untracked' })
    expect(wrapper.text()).toContain('No published end-of-life dates')
    expect(wrapper.find('[data-testid="timeline"]').exists()).toBe(false)
    expect(wrapper.find('p.font-black').exists()).toBe(false)
  })

  it('lists six items and folds the rest into a counter with their names', () => {
    const wrapper = mountRow({ items: items(9) })
    const chips = wrapper.findAll('ul > li')
    expect(chips.map((c) => c.text())).toEqual([...items(6).map((i) => i.label), '+3'])
    expect(chips[6]!.attributes('title')).toBe('item-6, item-7, item-8')
    expect(chips[0]!.attributes('title')).toBe('title-0')
  })

  it('shows no counter up to six items', () => {
    const wrapper = mountRow({ items: items(6) })
    expect(wrapper.findAll('ul > li')).toHaveLength(6)
  })

  it('shows the host of an item only when it carries one', () => {
    const wrapper = mountRow({
      items: [
        { key: 'a', label: 'db', agent: { id: 'agent-nas', hostname: 'nas-02', label: 'NAS cave' } },
        { key: 'b', label: 'cache' },
      ],
    })
    const badges = wrapper.findAllComponents(AgentBadge)
    expect(badges).toHaveLength(1)
    expect(badges[0]!.text()).toBe('NAS cave')
  })

  it('keeps the grid that collapses to two columns on a phone', () => {
    const classes = mountRow({}).find('li').classes()
    expect(classes).toContain('grid-cols-[minmax(0,1fr)_auto]')
    expect(classes.some((c) => c.startsWith('sm:grid-cols-'))).toBe(true)
  })
})
