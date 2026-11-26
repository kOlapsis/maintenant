import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import EndpointDetail from '@/components/EndpointDetail.vue'

const LOCAL = '00000000-0000-0000-0000-000000000000'

function endpoint(lastCheckAt: string, status: string) {
  return {
    id: 'e1',
    agent_id: LOCAL,
    container_name: 'web',
    label_key: 'k1',
    external_id: 'ctr',
    endpoint_type: 'http',
    target: 'https://slow.example.com',
    status,
    alert_state: 'normal',
    consecutive_failures: 0,
    consecutive_successes: 1,
    last_check_at: lastCheckAt,
    config: { interval: 30, timeout: 10, failure_threshold: 1, recovery_threshold: 1, method: 'GET', expected_status: '2xx', tls_verify: true, max_redirects: 5 },
    active: true,
    first_seen_at: '2026-11-01T10:00:00Z',
    last_seen_at: '2026-11-01T10:00:00Z',
  }
}

const { getEndpoint, checkEndpointNow, listChecks, applyEndpoint } = vi.hoisted(() => ({
  getEndpoint: vi.fn(),
  checkEndpointNow: vi.fn(),
  listChecks: vi.fn(),
  applyEndpoint: vi.fn(),
}))

vi.mock('@/services/endpointApi', () => ({ getEndpoint, checkEndpointNow, listChecks, deleteEndpoint: vi.fn() }))
vi.mock('@/services/uptimeApi', () => ({ fetchEndpointDailyUptime: vi.fn(async () => []) }))
vi.mock('@/stores/endpoints', () => ({ useEndpointsStore: () => ({ endpoints: [], applyEndpoint }) }))
vi.mock('@/composables/useConfirm', () => ({ useConfirm: () => vi.fn() }))

describe('EndpointDetail, a check that outlasts the request', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    for (const fn of [getEndpoint, checkEndpointNow, listChecks, applyEndpoint]) fn.mockReset()
    listChecks.mockResolvedValue({ endpoint_id: 'e1', checks: [], total: 0, has_more: false })
  })
  afterEach(() => vi.useRealTimers())

  it('says the check is still running, then shows its result', async () => {
    const before = endpoint('2026-11-26T10:00:00Z', 'up')
    const after = endpoint('2026-11-26T10:00:12Z', 'down')
    getEndpoint.mockResolvedValue({ endpoint: before })
    checkEndpointNow.mockResolvedValue({ endpoint: before, pending: true })

    const wrapper = mount(EndpointDetail, { props: { endpointId: 'e1' }, shallow: true, global: { stubs: { UiButton: false, InlineAlert: false } } })
    await flushPromises()
    const button = wrapper.findAll('button').find((b) => b.text().includes('Check now'))
    if (!button) throw new Error('the Check now button is missing')
    await button.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Check still running')
    expect(wrapper.text()).not.toContain('failed')
    expect(applyEndpoint).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(2000)
    expect(wrapper.text()).toContain('Check still running')

    getEndpoint.mockResolvedValue({ endpoint: after })
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()

    expect(wrapper.text()).not.toContain('Check still running')
    expect(applyEndpoint).toHaveBeenCalledWith(after)
  })
})
