// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { computed, defineComponent, h, type ComponentPublicInstance } from 'vue'
import { mount, flushPromises, type DOMWrapper, type VueWrapper } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import { createPinia, setActivePinia } from 'pinia'
import DefaultLayout from '@/layouts/DefaultLayout.vue'
import { useEdition } from '@/composables/useEdition'
import type { EditionResponse } from '@/services/editionApi'
import { editionPayload } from '@/components/__tests__/communityLimitFixtures'

// 051-community-label-limits, US4 scenarios 1, 5, 6 and 7, FR-016, FR-022, SC-006:
// the Community banner keeps its container tiers, its first text no longer claims
// a limit is reached, it announces the elements left out by the cap, and it comes
// back 7 days after a dismissal.

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
const api = vi.hoisted(() => ({ next: null as null | (() => Promise<EditionResponse>) }))
const sse = vi.hoisted(() => new Map<string, Array<(e: MessageEvent) => void>>())

vi.mock('@/services/editionApi', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/services/editionApi')>()),
  fetchEdition: vi.fn(() => (api.next ? api.next() : new Promise(() => {}))),
  fetchLicenseStatus: vi.fn(() => new Promise(() => {})),
}))
vi.mock('@/services/sseBus', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/sseBus')>()
  const on = (name: string, handler: (e: MessageEvent) => void) => {
    sse.set(name, [...(sse.get(name) ?? []), handler])
    actual.on(name, handler)
  }
  return { ...actual, on, sseBus: { ...actual.sseBus, on } }
})
vi.mock('@/stores/containers', () => ({
  useContainersStore: () => ({
    get containerCount() {
      return fleet.count
    },
    get groups() {
      if (fleet.count === 0) return []
      return [
        {
          name: 'stack',
          source: 'docker',
          containers: Array.from({ length: fleet.count }, (_, i) => ({
            id: String(i),
            archived: false,
          })),
        },
      ]
    },
    activeContainers: [],
  }),
}))
vi.mock('@/stores/swarm', () => ({ useSwarmStore: () => ({ loadInfo: vi.fn() }) }))
vi.mock('@/stores/runtime', () => ({
  useRuntimeStore: () => ({ fetchStatus: vi.fn(), startListening: vi.fn() }),
}))
vi.mock('@/stores/storage', () => ({
  useStorageStore: () => ({ fetchStatus: vi.fn(), startListening: vi.fn() }),
}))
vi.mock('@/stores/resources', () => ({
  useResourcesStore: () => ({ selected: null, entityQuery: undefined }),
}))
vi.mock('@/composables/useFleetRuntimes', () => ({
  useFleetRuntimes: () => ({ availableRuntimes: computed(() => ['docker']) }),
}))
vi.mock('@/composables/useAppVersion', async () => {
  const { ref } = await import('vue')
  return { useAppVersion: () => ({ version: ref('v1.6.0') }) }
})

const Blank = defineComponent({ render: () => h('div') })

const T0 = new Date('2026-11-06T09:00:00Z').getTime()
const MINUTE = 60 * 1000
const HOUR = 60 * MINUTE
const DAY = 24 * HOUR

const tierOne = (hosts: number) =>
  `Pro adds team alerting at €29/mo. Personal monitors ${hosts} machines for €149 once, for life.`
const tierTwo = (n: number, hosts = 20) =>
  `You're monitoring ${n} containers. Pro adds team alerting at €29/mo, Personal ${hosts} machines for €149 once.`
const TIER_THREE =
  '50+ containers in production? Pro adds escalation, incidents and Slack routing. Talk to the founder?'
const CONTAINER_MESSAGE =
  /Pro adds team alerting|Personal lifts every Community limit|Hitting the Community limits|You're monitoring \d+ containers|containers in production/

type Layout = VueWrapper<ComponentPublicInstance>
type Left = { endpoints?: number; certificates?: number }

function community(left: Left = {}, personalHosts = 20): EditionResponse {
  const payload = editionPayload('community', {
    endpoints: { used: 10, limit: 10, unmonitored: left.endpoints ?? 0 },
    certificates: { used: 5, limit: 5, unmonitored: left.certificates ?? 0 },
  })
  payload.tiers!.personal!.agent_hosts = personalHosts
  return payload
}

const flat = (s: string) => s.replace(/\s+/g, ' ').trim()

async function mountLayout(): Promise<Layout> {
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

function banners(wrapper: Layout): DOMWrapper<Element>[] {
  return wrapper.findAll('[role="alert"], [role="status"]')
}

function containerBanner(wrapper: Layout): DOMWrapper<Element> | undefined {
  return banners(wrapper).find((b) => CONTAINER_MESSAGE.test(flat(b.text())))
}

function leftOutBanner(wrapper: Layout): DOMWrapper<Element> | undefined {
  return banners(wrapper).find((b) => /not monitored/i.test(b.text()))
}

function editionsLinks(banner: DOMWrapper<Element>): number {
  return banner.findAll('a').filter((a) => a.attributes('href') === '/editions').length
}

async function dismiss(banner: DOMWrapper<Element> | undefined): Promise<void> {
  expect(banner).toBeDefined()
  const button = banner!
    .findAll('button')
    .find((b) => /dismiss|close/i.test(b.attributes('aria-label') ?? ''))
  expect(button).toBeDefined()
  await button!.trigger('click')
  await flushPromises()
}

function fire(event: string, data: Record<string, unknown>): void {
  for (const handler of sse.get(event) ?? []) {
    handler(new MessageEvent(event, { data: JSON.stringify(data) }))
  }
}

function counted(n: number, noun: string): RegExp {
  return new RegExp(`(?<!\\d)${n} ${noun}${n === 1 ? '' : 's'}\\b`)
}

beforeEach(() => {
  setActivePinia(createPinia())
  localStorage.clear()
  api.next = null
  useEdition().licenseStatus.value = null
})

describe('DefaultLayout on Community, banner by containers', () => {
  // US4 scenario 1, FR-016, SC-006: the first tier presents Personal and no longer claims a limit is reached.
  for (const [count, hosts] of [
    [10, 20],
    [15, 20],
    [24, 50],
  ] as const) {
    it(`presents Personal without saying a limit is reached at ${count} containers (${hosts} machines)`, async () => {
      fleet.count = count
      useEdition().edition.value = community({}, hosts)
      const wrapper = await mountLayout()
      const text = flat(wrapper.text())

      expect(text).toContain(tierOne(hosts))
      expect(text).not.toMatch(/Hitting|limits? (is |are )?reached|you('ve| have) hit/i)
      const banner = containerBanner(wrapper)
      expect(banner).toBeDefined()
      expect(editionsLinks(banner!)).toBeGreaterThan(0)
      expect(leftOutBanner(wrapper)).toBeUndefined()
    })
  }

  // FR-016: the other tiers keep their current texts.
  for (const [count, hosts] of [
    [25, 20],
    [30, 20],
    [49, 50],
  ] as const) {
    it(`keeps the second tier text at ${count} containers (${hosts} machines)`, async () => {
      fleet.count = count
      useEdition().edition.value = community({}, hosts)
      const wrapper = await mountLayout()

      expect(flat(wrapper.text())).toContain(tierTwo(count, hosts))
      expect(editionsLinks(containerBanner(wrapper)!)).toBeGreaterThan(0)
    })
  }

  for (const count of [50, 88]) {
    it(`keeps the third tier text at ${count} containers`, async () => {
      fleet.count = count
      useEdition().edition.value = community()
      const wrapper = await mountLayout()

      expect(flat(wrapper.text())).toContain(TIER_THREE)
      expect(containerBanner(wrapper)!.find('a[href^="mailto:"]').exists()).toBe(true)
    })
  }

  it('shows no edition banner under 10 containers when nothing is left out', async () => {
    fleet.count = 9
    useEdition().edition.value = community()
    const wrapper = await mountLayout()
    const text = flat(wrapper.text())

    expect(text).not.toMatch(CONTAINER_MESSAGE)
    expect(text).not.toMatch(/€149|€29/)
    expect(leftOutBanner(wrapper)).toBeUndefined()
  })

  // FR-016: the licence state banner does not change.
  it('still shows the licence banner', async () => {
    fleet.count = 88
    useEdition().edition.value = community()
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

describe('DefaultLayout on a paid edition', () => {
  // US4 scenario 5: nothing changes, no edition banner.
  for (const edition of ['personal', 'pro'] as const) {
    it(`shows no edition banner on ${edition} with 30 containers`, async () => {
      fleet.count = 30
      useEdition().edition.value = editionPayload(edition, {
        endpoints: { used: 30, limit: -1, unmonitored: 0 },
        certificates: { used: 9, limit: -1, unmonitored: 0 },
      })
      const wrapper = await mountLayout()
      const text = flat(wrapper.text())

      expect(text).not.toMatch(CONTAINER_MESSAGE)
      expect(text).not.toMatch(/€149|€29/)
      expect(leftOutBanner(wrapper)).toBeUndefined()
    })
  }
})

describe('DefaultLayout on Community, banner for the elements left out', () => {
  // US4 scenario 6, FR-022: the counts, a zero omitted, Personal named, a link to
  // the Editions page, in place of the container message, whatever the container count.
  for (const c of [
    { containers: 15, endpoints: 3, certificates: 1 },
    { containers: 3, endpoints: 20, certificates: 0 },
    { containers: 0, endpoints: 0, certificates: 2 },
    { containers: 88, endpoints: 1, certificates: 4 },
  ]) {
    it(`announces ${c.endpoints} endpoints and ${c.certificates} certificates left out with ${c.containers} containers`, async () => {
      fleet.count = c.containers
      useEdition().edition.value = community({
        endpoints: c.endpoints,
        certificates: c.certificates,
      })
      const wrapper = await mountLayout()

      const banner = leftOutBanner(wrapper)
      expect(banner).toBeDefined()
      const text = flat(banner!.text())
      const said = [
        ...(c.endpoints > 0 ? [counted(c.endpoints, 'endpoint')] : []),
        ...(c.certificates > 0 ? [counted(c.certificates, 'certificate')] : []),
      ]
      for (const count of said) expect(text).toMatch(count)
      expect(text).not.toMatch(/(?<!\d)0\s+(endpoint|certificate)/i)
      expect(text).toMatch(/Community limit/)
      expect(text).toContain('Personal')
      expect(text).not.toMatch(/€/)
      expect(editionsLinks(banner!)).toBeGreaterThan(0)
      expect(flat(wrapper.text())).not.toMatch(CONTAINER_MESSAGE)
    })
  }

  // FR-022, SC-006: the banner follows the limit events without a page reload.
  it('follows the limit events without a page reload', async () => {
    fleet.count = 30
    useEdition().edition.value = community()
    const wrapper = await mountLayout()
    expect(flat(wrapper.text())).toContain(tierTwo(30))
    expect(leftOutBanner(wrapper)).toBeUndefined()

    api.next = () => Promise.resolve(community({ endpoints: 3, certificates: 1 }))
    fire('endpoint.limit_changed', { endpoint_id: 'ep-svc-11', limited: true })
    await vi.waitFor(() => expect(leftOutBanner(wrapper)).toBeDefined(), {
      timeout: 2000,
    })
    const text = flat(leftOutBanner(wrapper)!.text())
    expect(text).toMatch(counted(3, 'endpoint'))
    expect(text).toMatch(counted(1, 'certificate'))
    expect(flat(wrapper.text())).not.toMatch(CONTAINER_MESSAGE)

    api.next = () => Promise.resolve(community())
    fire('certificate.limit_changed', { monitor_id: 'cert-06', limited: false })
    await vi.waitFor(() => expect(leftOutBanner(wrapper)).toBeUndefined(), { timeout: 2000 })
    expect(flat(wrapper.text())).toContain(tierTwo(30))
  })
})

describe('DefaultLayout on Community, dismissal', () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(T0)
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  // FR-016, US4 scenario 7: the container message comes back after 7 days, not before.
  for (const [count, message] of [
    [30, tierTwo(30)],
    [88, TIER_THREE],
  ] as const) {
    it(`brings the container message back 7 days after a dismissal at ${count} containers`, async () => {
      fleet.count = count
      useEdition().edition.value = community()
      let wrapper = await mountLayout()
      expect(flat(wrapper.text())).toContain(message)
      await dismiss(containerBanner(wrapper))
      expect(flat(wrapper.text())).not.toContain(message)
      wrapper.unmount()

      vi.setSystemTime(T0 + 7 * DAY - HOUR)
      wrapper = await mountLayout()
      expect(flat(wrapper.text())).not.toContain(message)
      wrapper.unmount()

      vi.setSystemTime(T0 + 7 * DAY + MINUTE)
      wrapper = await mountLayout()
      expect(flat(wrapper.text())).toContain(message)
    })
  }

  // FR-022, US4 scenario 7: the message of the elements left out comes back after 7 days, not before.
  for (const c of [
    { containers: 3, endpoints: 3, certificates: 1 },
    { containers: 0, endpoints: 0, certificates: 2 },
  ]) {
    it(`brings the left-out message back 7 days after a dismissal with ${c.containers} containers`, async () => {
      fleet.count = c.containers
      useEdition().edition.value = community({
        endpoints: c.endpoints,
        certificates: c.certificates,
      })
      let wrapper = await mountLayout()
      await dismiss(leftOutBanner(wrapper))
      expect(leftOutBanner(wrapper)).toBeUndefined()
      wrapper.unmount()

      vi.setSystemTime(T0 + 7 * DAY - HOUR)
      wrapper = await mountLayout()
      expect(leftOutBanner(wrapper)).toBeUndefined()
      wrapper.unmount()

      vi.setSystemTime(T0 + 7 * DAY + MINUTE)
      wrapper = await mountLayout()
      expect(leftOutBanner(wrapper)).toBeDefined()
    })
  }

  // FR-022, US4 scenario 7: dismissing the container message does not hide the left-out message.
  for (const count of [15, 30]) {
    it(`keeps the left-out message after the container message is dismissed at ${count} containers`, async () => {
      fleet.count = count
      useEdition().edition.value = community()
      let wrapper = await mountLayout()
      await dismiss(containerBanner(wrapper))

      useEdition().edition.value = community({ endpoints: 2 })
      await flushPromises()
      expect(leftOutBanner(wrapper)).toBeDefined()
      wrapper.unmount()

      vi.setSystemTime(T0 + DAY)
      wrapper = await mountLayout()
      expect(leftOutBanner(wrapper)).toBeDefined()
      expect(flat(leftOutBanner(wrapper)!.text())).toMatch(counted(2, 'endpoint'))
    })
  }
})
