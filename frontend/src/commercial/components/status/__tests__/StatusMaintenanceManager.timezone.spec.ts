import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import type { MaintenanceWindow } from '@/services/statusApi'
import StatusMaintenanceManager from '@/commercial/components/status/StatusMaintenanceManager.vue'

process.env.TZ = 'Europe/Paris'

const { updateMaintenance, maintenance } = vi.hoisted(() => ({
  updateMaintenance: vi.fn(),
  maintenance: [] as MaintenanceWindow[],
}))

vi.mock('@/services/statusApi', () => ({
  updateMaintenance,
  createMaintenance: vi.fn(),
  deleteMaintenance: vi.fn(),
}))

vi.mock('@/stores/statusAdmin', () => ({
  useStatusAdminStore: () => ({
    components: [],
    maintenance,
    maintenanceLoading: false,
    fetchMaintenance: vi.fn(),
  }),
}))

vi.mock('@/composables/useConfirm', () => ({ useConfirm: () => vi.fn() }))

function makeWindow(starts_at: string, ends_at: string): MaintenanceWindow {
  return {
    id: 'mw-1',
    title: 'Database upgrade',
    description: '',
    starts_at,
    ends_at,
    active: false,
    components: [],
    created_at: '2098-12-01T00:00:00Z',
  }
}

describe('StatusMaintenanceManager time zone', () => {
  beforeEach(() => {
    updateMaintenance.mockReset()
    updateMaintenance.mockResolvedValue({})
    maintenance.length = 0
  })

  it.each([
    ['winter', '2099-12-01T10:00:00Z', '2099-12-01T12:00:00Z'],
    ['summer', '2099-07-01T10:00:00Z', '2099-07-01T12:00:00Z'],
  ])('saving a %s window unchanged keeps its start and end', async (_season, starts, ends) => {
    maintenance.push(makeWindow(starts, ends))
    const wrapper = mount(StatusMaintenanceManager)
    await wrapper.findAll('button').find((b) => b.text() === 'Edit')!.trigger('click')

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    const [, data] = updateMaintenance.mock.calls[0]!
    expect(data.starts_at).toBe(new Date(starts).toISOString())
    expect(data.ends_at).toBe(new Date(ends).toISOString())
  })
})
