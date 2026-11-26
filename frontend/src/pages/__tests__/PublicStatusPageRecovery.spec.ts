import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import PublicStatusPage from '@/pages/PublicStatusPage.vue'
import { sessionExpired } from '@/services/authGuard'

const fetchMock = vi.fn()
vi.stubGlobal('fetch', fetchMock)

class FakeEventSource {
  static instances: FakeEventSource[] = []
  listeners = new Map<string, () => void>()
  static CLOSED = 2
  readyState = 1
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
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
  fetchMock.mockImplementation(async (url: string) => {
    if (url !== '/status/api') return response(404, {})
    const status = statuses[Math.min(n++, statuses.length - 1)]!
    return response(status, status === 200 ? snapshot : {})
  })
}

function statusCalls() {
  return fetchMock.mock.calls.filter(([url]) => url === '/status/api').length
}

describe('PublicStatusPage recovery', () => {
  beforeEach(() => {
    fetchMock.mockReset()
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

  it('says it could not refresh and how old the data is', async () => {
    vi.useFakeTimers()
    serve(200, 503)
    const wrapper = mount(PublicStatusPage)
    await vi.advanceTimersByTimeAsync(0)
    expect(wrapper.text()).not.toContain('Could not refresh')

    await vi.advanceTimersByTimeAsync(5 * 60_000)

    expect(wrapper.text()).toContain('Could not refresh')
    expect(wrapper.text()).toContain('Updated 5 minutes ago')
  })

  it('reopens a stream the server closed, waiting longer each time', async () => {
    vi.useFakeTimers()
    serve(200)
    mount(PublicStatusPage)
    await vi.advanceTimersByTimeAsync(0)

    const first = FakeEventSource.instances[0]!
    first.onopen!()
    first.readyState = FakeEventSource.CLOSED
    first.onerror!()
    await vi.advanceTimersByTimeAsync(999)
    expect(FakeEventSource.instances).toHaveLength(1)
    await vi.advanceTimersByTimeAsync(1)
    expect(FakeEventSource.instances).toHaveLength(2)

    const second = FakeEventSource.instances[1]!
    second.readyState = FakeEventSource.CLOSED
    second.onerror!()
    await vi.advanceTimersByTimeAsync(1_000)
    expect(FakeEventSource.instances).toHaveLength(2)
    await vi.advanceTimersByTimeAsync(1_000)
    expect(FakeEventSource.instances).toHaveLength(3)

    const initial = statusCalls()
    FakeEventSource.instances[2]!.onopen!()
    await vi.advanceTimersByTimeAsync(0)
    expect(statusCalls()).toBe(initial + 1)
  })

  it('never takes a refused answer for an expired session', async () => {
    fetchMock.mockImplementation(async () => ({ ok: false, status: 403, redirected: true, json: async () => { throw new Error('not json') } }))
    const wrapper = mount(PublicStatusPage)
    await flushPromises()

    expect(wrapper.text()).toContain('HTTP 403')
    expect(sessionExpired.value).toBe(false)
  })
})
