import { describe, it, expect, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import WebhooksPage from '@/pages/WebhooksPage.vue'
import type { WebhookSubscription } from '@/services/webhookApi'

const hook = (over: Partial<WebhookSubscription>): WebhookSubscription => ({
  id: 'w1',
  name: 'ci',
  url: 'https://hooks.example.com/ci',
  event_types: ['*'],
  is_active: true,
  failure_count: 0,
  created_at: '2026-09-01T10:00:00Z',
  ...over,
})

const listWebhooks = vi.fn()
const testWebhook = vi.fn()

vi.mock('@/services/webhookApi', () => ({
  listWebhooks: () => listWebhooks(),
  testWebhook: (id: string) => testWebhook(id),
  deleteWebhook: vi.fn(),
}))

vi.mock('@/composables/useConfirm', () => ({
  useConfirm: () => vi.fn(),
}))

describe('WebhooksPage', () => {
  it('shows the delivery state the test left behind', async () => {
    listWebhooks
      .mockResolvedValueOnce({ webhooks: [hook({ is_active: false, failure_count: 10, last_delivery_status: 'failed' })] })
      .mockResolvedValueOnce({ webhooks: [hook({ last_delivery_status: 'delivered' })] })
    testWebhook.mockResolvedValue({ status: 'delivered', http_status: 200 })

    const wrapper = mount(WebhooksPage, {
      global: { stubs: { FeatureHint: true, WebhookForm: true, 'router-link': true } },
    })
    await flushPromises()
    expect(wrapper.text()).toContain('Disabled')
    expect(wrapper.text()).toContain('(10 failures)')

    const testButton = wrapper.findAll('button').find((b) => b.text() === 'Test')
    expect(testButton).toBeDefined()
    await testButton!.trigger('click')
    await flushPromises()

    expect(testWebhook).toHaveBeenCalledWith('w1')
    expect(listWebhooks).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('Active')
    expect(wrapper.text()).not.toContain('failures)')
  })
})
