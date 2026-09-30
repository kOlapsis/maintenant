import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import PublicStatusPage from '@/pages/PublicStatusPage.vue'

const { guardedFetch } = vi.hoisted(() => ({ guardedFetch: vi.fn() }))

vi.mock('@/services/apiFetch', () => ({ guardedFetch }))

class FakeEventSource {
  addEventListener() {}
  close() {}
}
vi.stubGlobal('EventSource', FakeEventSource)

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
