import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises, RouterLinkStub } from '@vue/test-utils'
import ChannelManager from '@/components/ChannelManager.vue'
import { ApiError } from '@/services/apiFetch'

const { updateChannel, deleteChannel, testChannel, fetchChannels, confirm } = vi.hoisted(() => ({
  updateChannel: vi.fn(),
  deleteChannel: vi.fn(),
  testChannel: vi.fn(),
  fetchChannels: vi.fn(),
  confirm: vi.fn(),
}))

vi.mock('@/services/alertApi', () => ({
  createChannel: vi.fn(),
  updateChannel,
  deleteChannel,
  testChannel,
}))

vi.mock('@/stores/channels', () => ({
  useChannelsStore: () => ({
    channels: [
      {
        id: 'c1',
        name: 'ops',
        type: 'webhook',
        url: 'https://hooks.example.com/x',
        headers: '',
        enabled: true,
        health: 'healthy',
        created_at: '2026-10-21T10:00:00Z',
        updated_at: '2026-10-21T10:00:00Z',
      },
    ],
    channelsLoading: false,
    fetchChannels,
  }),
}))

vi.mock('@/composables/useConfirm', () => ({ useConfirm: () => confirm }))

function mountManager() {
  return mount(ChannelManager, { global: { stubs: { ChannelWizard: true, RouterLink: RouterLinkStub } } })
}

function button(wrapper: ReturnType<typeof mountManager>, label: string) {
  const b = wrapper.findAll('button').find((x) => x.text() === label)
  if (!b) throw new Error(`button "${label}" is missing`)
  return b
}

describe('ChannelManager failures', () => {
  beforeEach(() => {
    for (const fn of [updateChannel, deleteChannel, testChannel, fetchChannels, confirm]) fn.mockReset()
    confirm.mockResolvedValue(true)
  })

  it('sends one request for two quick saves', async () => {
    updateChannel.mockResolvedValue({})
    const wrapper = mountManager()
    await button(wrapper, 'Edit').trigger('click')
    const form = wrapper.find('form')
    void form.trigger('submit')
    void form.trigger('submit')
    await flushPromises()
    expect(updateChannel).toHaveBeenCalledOnce()
  })

  it('shows the error when deleting fails', async () => {
    deleteChannel.mockRejectedValue(new Error('HTTP 403'))
    const wrapper = mountManager()
    await button(wrapper, 'Delete').trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toContain('HTTP 403')
  })

  it('names the escalation policies that keep a channel from being deleted', async () => {
    deleteChannel.mockRejectedValue(
      new ApiError(409, { code: 'CHANNEL_IN_USE', message: 'channel is used by escalation policies: night shift' }, 'channel is used by escalation policies: night shift'),
    )
    const wrapper = mountManager()
    await button(wrapper, 'Delete').trigger('click')
    await flushPromises()
    expect(confirm.mock.calls[0]![0].message).not.toContain('escalation policies referencing')
    expect(confirm.mock.calls[0]![0].message).toContain('cannot be deleted')
    const alert = wrapper.find('[role="alert"]').text()
    expect(alert).toContain('night shift')
    expect(alert).toContain('Remove the channel from them')
  })

  it('sends one request for two quick deletes', async () => {
    deleteChannel.mockResolvedValue(undefined)
    const wrapper = mountManager()
    const del = button(wrapper, 'Delete')
    void del.trigger('click')
    void del.trigger('click')
    await flushPromises()
    expect(deleteChannel).toHaveBeenCalledOnce()
  })

  it('shows the error when the test request fails', async () => {
    testChannel.mockRejectedValue(new Error('HTTP 502'))
    const wrapper = mountManager()
    await button(wrapper, 'Test').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('HTTP 502')
  })

  it('sends one request for two quick tests', async () => {
    testChannel.mockResolvedValue({ status: 'delivered', response_code: 200 })
    const wrapper = mountManager()
    const test = button(wrapper, 'Test')
    void test.trigger('click')
    void test.trigger('click')
    await flushPromises()
    expect(testChannel).toHaveBeenCalledOnce()
  })
})
