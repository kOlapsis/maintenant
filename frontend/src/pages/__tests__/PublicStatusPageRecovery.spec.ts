import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import PublicStatusPage from '@/pages/PublicStatusPage.vue'

const { guardedFetch } = vi.hoisted(() => ({ guardedFetch: vi.fn() }))

vi.mock('@/services/apiFetch', () => ({ guardedFetch }))

class FakeEventSource {
  static instances: FakeEventSource[] = []
  listeners = new Map<string, () => void>()
  onopen: (() => void) | null = null
  constructor() {
    FakeEventSource.instances.push(this)
  }
  addEventListener(name: string, fn: () => void) {
    this.listeners.set(name, fn)
  }
  close() {}
}
vi.stubGlobal('EventSource', FakeEventSource)

function response(status: number, body: unknown) {
  return { ok: status >= 200 && status < 300, status, json: async () => body }
}

const snapshot = {
  global_status: 'operational',
  global_message: 'All Systems Operational',
  updated_at: '2026-11-19T10:00:00Z',
  components: [],
  active_incidents: [],
  upcoming_maintenance: [],
  subscriptions_enabled: false,
}

function serve(...statuses: number[]) {
  let n = 0
  guardedFetch.mockImplementation(async (url: string) => {
    if (url !== '/status/api') return response(404, {})
    const status = statuses[Math.min(n++, statuses.length - 1)]!
    return response(status, status === 200 ? snapshot : {})
  })
}

function statusCalls() {
  return guardedFetch.mock.calls.filter(([url]) => url === '/status/api').length
}

describe('PublicStatusPage recovery', () => {
  beforeEach(() => {
    guardedFetch.mockReset()
    FakeEventSource.instances = []
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('shows the data again once a later load succeeds', async () => {
    serve(503, 200)
    const wrapper = mount(PublicStatusPage)
    await flushPromises()
    expect(wrapper.text()).toContain('HTTP 503')

    FakeEventSource.instances[0]!.listeners.get('status.incident_created')!()
    await flushPromises()

    expect(wrapper.text()).not.toContain('HTTP 503')
    expect(wrapper.text()).toContain('All Systems Operational')
  })

  it('keeps showing the data when a later load fails', async () => {
    serve(200, 503)
    const wrapper = mount(PublicStatusPage)
    await flushPromises()

    FakeEventSource.instances[0]!.listeners.get('status.incident_created')!()
    await flushPromises()

    expect(wrapper.text()).toContain('All Systems Operational')
  })

  it('reloads when the stream reopens, not on its first opening', async () => {
    serve(200)
    mount(PublicStatusPage)
    await flushPromises()
    const es = FakeEventSource.instances[0]!
    const initial = statusCalls()

    es.onopen!()
    await flushPromises()
    expect(statusCalls()).toBe(initial)

    es.onopen!()
    await flushPromises()
    expect(statusCalls()).toBe(initial + 1)
  })

  it('reloads on a slow timer as a fallback', async () => {
    vi.useFakeTimers()
    serve(200)
    mount(PublicStatusPage)
    await vi.advanceTimersByTimeAsync(0)
    const initial = statusCalls()

    await vi.advanceTimersByTimeAsync(60_000)

    expect(statusCalls()).toBe(initial + 1)
  })
})
