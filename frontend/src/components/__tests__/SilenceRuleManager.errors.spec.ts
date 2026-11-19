import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import SilenceRuleManager from '@/components/SilenceRuleManager.vue'

const { createSilenceRule, cancelSilenceRule, fetchSilenceRules, confirm } = vi.hoisted(() => ({
  createSilenceRule: vi.fn(),
  cancelSilenceRule: vi.fn(),
  fetchSilenceRules: vi.fn(),
  confirm: vi.fn(),
}))

vi.mock('@/services/alertApi', () => ({ createSilenceRule, cancelSilenceRule }))

vi.mock('@/stores/alerts', () => ({
  useAlertsStore: () => ({
    silenceRules: [
      {
        id: 'sr-1',
        source: 'container',
        entity_type: '',
        entity_id: '',
        reason: '',
        duration_seconds: 900,
        is_active: true,
        starts_at: '2026-11-01T10:00:00Z',
        expires_at: '2026-11-01T10:15:00Z',
      },
    ],
    silenceLoading: false,
    fetchSilenceRules,
  }),
}))

vi.mock('@/composables/useConfirm', () => ({ useConfirm: () => confirm }))

function button(wrapper: ReturnType<typeof mount>, label: string) {
  const b = wrapper.findAll('button').find((x) => x.text() === label)
  if (!b) throw new Error(`button "${label}" is missing`)
  return b
}

describe('SilenceRuleManager failures', () => {
  beforeEach(() => {
    for (const fn of [createSilenceRule, cancelSilenceRule, fetchSilenceRules, confirm]) fn.mockReset()
    confirm.mockResolvedValue(true)
  })

  it('shows the error when creating fails and keeps the form open', async () => {
    createSilenceRule.mockRejectedValue(new Error('HTTP 500'))
    const wrapper = mount(SilenceRuleManager)
    await button(wrapper, 'Create Silence Rule').trigger('click')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toContain('HTTP 500')
    expect(wrapper.find('form').exists()).toBe(true)
  })

  it('sends one request for two quick submits', async () => {
    createSilenceRule.mockResolvedValue({})
    const wrapper = mount(SilenceRuleManager)
    await button(wrapper, 'Create Silence Rule').trigger('click')
    const form = wrapper.find('form')
    void form.trigger('submit')
    void form.trigger('submit')
    await flushPromises()
    expect(createSilenceRule).toHaveBeenCalledOnce()
  })

  it('shows the error when cancelling fails', async () => {
    cancelSilenceRule.mockRejectedValue(new Error('HTTP 403'))
    const wrapper = mount(SilenceRuleManager)
    await button(wrapper, 'Cancel').trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toContain('HTTP 403')
  })

  it('sends one request for two quick cancels', async () => {
    cancelSilenceRule.mockResolvedValue(undefined)
    const wrapper = mount(SilenceRuleManager)
    const cancel = button(wrapper, 'Cancel')
    void cancel.trigger('click')
    void cancel.trigger('click')
    await flushPromises()
    expect(cancelSilenceRule).toHaveBeenCalledOnce()
  })
})
