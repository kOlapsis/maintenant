import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import StatusMaintenanceManager from '@/commercial/components/status/StatusMaintenanceManager.vue'

const { createMaintenance, updateMaintenance, deleteMaintenance, fetchMaintenance, confirm } = vi.hoisted(() => ({
  createMaintenance: vi.fn(),
  updateMaintenance: vi.fn(),
  deleteMaintenance: vi.fn(),
  fetchMaintenance: vi.fn(),
  confirm: vi.fn(),
}))

vi.mock('@/services/statusApi', () => ({ createMaintenance, updateMaintenance, deleteMaintenance }))

vi.mock('@/stores/statusAdmin', () => ({
  useStatusAdminStore: () => ({
    components: [],
    maintenance: [
      {
        id: 'mw-1',
        title: 'Upgrade',
        description: '',
        starts_at: '2099-01-01T02:00:00Z',
        ends_at: '2099-01-01T04:00:00Z',
        active: false,
        components: [],
        created_at: '2098-12-01T00:00:00Z',
      },
    ],
    maintenanceLoading: false,
    fetchMaintenance,
  }),
}))

vi.mock('@/composables/useConfirm', () => ({ useConfirm: () => confirm }))

function button(wrapper: ReturnType<typeof mount>, label: string) {
  const b = wrapper.findAll('button').find((x) => x.text() === label)
  if (!b) throw new Error(`button "${label}" is missing`)
  return b
}

describe('StatusMaintenanceManager failures', () => {
  beforeEach(() => {
    for (const fn of [createMaintenance, updateMaintenance, deleteMaintenance, fetchMaintenance, confirm]) fn.mockReset()
    confirm.mockResolvedValue(true)
  })

  it('shows the error when saving fails and keeps the form open', async () => {
    updateMaintenance.mockRejectedValue(new Error('HTTP 500'))
    const wrapper = mount(StatusMaintenanceManager)
    await button(wrapper, 'Edit').trigger('click')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toContain('HTTP 500')
    expect(wrapper.find('form').exists()).toBe(true)
  })

  it('sends one request for two quick submits', async () => {
    updateMaintenance.mockResolvedValue({})
    const wrapper = mount(StatusMaintenanceManager)
    await button(wrapper, 'Edit').trigger('click')
    const form = wrapper.find('form')
    void form.trigger('submit')
    void form.trigger('submit')
    await flushPromises()
    expect(updateMaintenance).toHaveBeenCalledOnce()
  })

  it('shows the error when deleting fails', async () => {
    deleteMaintenance.mockRejectedValue(new Error('HTTP 403'))
    const wrapper = mount(StatusMaintenanceManager)
    await button(wrapper, 'Delete').trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toContain('HTTP 403')
  })

  it('sends one request for two quick deletes', async () => {
    deleteMaintenance.mockResolvedValue(undefined)
    const wrapper = mount(StatusMaintenanceManager)
    const del = button(wrapper, 'Delete')
    void del.trigger('click')
    void del.trigger('click')
    await flushPromises()
    expect(deleteMaintenance).toHaveBeenCalledOnce()
  })
})
