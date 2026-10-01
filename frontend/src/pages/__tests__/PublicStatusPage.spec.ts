import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import PublicStatusPage from '@/pages/PublicStatusPage.vue'

const { guardedFetch } = vi.hoisted(() => ({ guardedFetch: vi.fn() }))

vi.mock('@/services/apiFetch', () => ({ guardedFetch }))

const listeners = new Map<string, (e: Event) => void>()

class FakeEventSource {
  addEventListener(name: string, fn: (e: Event) => void) {
    listeners.set(name, fn)
  }
  close() {}
}
vi.stubGlobal('EventSource', FakeEventSource)

function emit(name: string, payload: unknown) {
  listeners.get(name)?.(new MessageEvent(name, { data: JSON.stringify(payload) }))
}

function jsonResponse(status: number, body: unknown) {
  return { ok: status >= 200 && status < 300, status, json: async () => body }
}

function statusSnapshot(subscriptionsEnabled: boolean) {
  return {
    global_status: 'operational',
    global_message: 'All Systems Operational',
    updated_at: new Date().toISOString(),
    components: [],
    active_incidents: [],
    upcoming_maintenance: [],
    subscriptions_enabled: subscriptionsEnabled,
  }
}

function serveStatus(subscriptionsEnabled: boolean, subscribe = jsonResponse(200, { status: 'confirmation_sent' })) {
  guardedFetch.mockImplementation(async (url: string) => {
    if (url === '/status/api') return jsonResponse(200, statusSnapshot(subscriptionsEnabled))
    if (url === '/status/subscribe') return subscribe
    return jsonResponse(404, {})
  })
}

async function mountPage() {
  const wrapper = mount(PublicStatusPage)
  await flushPromises()
  return wrapper
}

describe('PublicStatusPage subscription form', () => {
  beforeEach(() => {
    guardedFetch.mockReset()
  })

  it('offers no subscription while the server cannot send the emails', async () => {
    serveStatus(false)
    const wrapper = await mountPage()

    expect(wrapper.find('input[type="email"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('Subscribe to updates')
  })

  it('posts the address and asks the visitor to confirm from the email', async () => {
    serveStatus(true)
    const wrapper = await mountPage()

    await wrapper.find('input[type="email"]').setValue('visitor@example.com')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    const call = guardedFetch.mock.calls.find(([url]) => url === '/status/subscribe')
    expect(call).toBeDefined()
    expect(call?.[1]).toMatchObject({ method: 'POST', body: JSON.stringify({ email: 'visitor@example.com' }) })
    expect(wrapper.text()).toContain('Check your inbox')
  })

  it('explains a refusal from its error code', async () => {
    serveStatus(true, jsonResponse(429, { error: { code: 'rate_limited', message: 'Too many requests' } }))
    const wrapper = await mountPage()

    await wrapper.find('input[type="email"]').setValue('visitor@example.com')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('Too many attempts')
    expect(wrapper.text()).not.toContain('Check your inbox')
  })
})

describe('PublicStatusPage live updates', () => {
  beforeEach(() => {
    guardedFetch.mockReset()
    guardedFetch.mockImplementation(async (url: string) => {
      if (url !== '/status/api') return jsonResponse(404, {})
      return jsonResponse(200, {
        ...statusSnapshot(false),
        components: [{ id: 'c1', name: 'API', status: 'operational' }],
      })
    })
  })

  const statusCalls = () => guardedFetch.mock.calls.filter(([url]) => url === '/status/api').length

  it('updates a component and the banner in place', async () => {
    const wrapper = await mountPage()
    expect(wrapper.text()).toContain('Operational')

    emit('status.component_changed', {
      component_id: 'c1',
      name: 'Public API',
      status: 'major_outage',
      monitors: [{ type: 'endpoint', id: 'e1', name: 'https://api.example.com', status: 'major_outage' }],
    })
    emit('status.global_changed', { status: 'major_outage', message: 'Major Outage' })
    await flushPromises()

    expect(wrapper.text()).toContain('Public API')
    expect(wrapper.text()).toContain('Major Outage')
    expect(wrapper.find('[aria-controls="breakdown-c1"]').exists()).toBe(true)
    expect(statusCalls()).toBe(1)
  })

  it('offers the monitor breakdown from the first load', async () => {
    guardedFetch.mockImplementation(async (url: string) => {
      if (url !== '/status/api') return jsonResponse(404, {})
      return jsonResponse(200, {
        ...statusSnapshot(false),
        components: [{
          id: 'c1', name: 'API', status: 'degraded',
          monitors: [{ type: 'endpoint', id: 'e1', name: 'https://api.example.com', status: 'degraded' }],
        }],
      })
    })
    const wrapper = await mountPage()

    await wrapper.find('[aria-controls="breakdown-c1"]').trigger('click')

    expect(wrapper.find('#breakdown-c1').text()).toContain('https://api.example.com')
  })

  it('ignores a component the page does not show', async () => {
    const wrapper = await mountPage()
    emit('status.component_changed', { component_id: 'hidden', name: 'Internal', status: 'major_outage', monitors: null })
    await flushPromises()

    expect(wrapper.text()).not.toContain('Internal')
    expect(statusCalls()).toBe(1)
  })

  it('reloads when a component is added, edited or removed', async () => {
    await mountPage()
    emit('status.component_created', { component_id: 'c2' })
    await flushPromises()

    expect(statusCalls()).toBe(2)
  })
})
