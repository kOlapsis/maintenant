import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import type { MaintenanceWindow } from '@/services/statusApi'
import StatusMaintenanceManager from '@/commercial/components/status/StatusMaintenanceManager.vue'

const { updateMaintenance, fetchMaintenance } = vi.hoisted(() => ({
  updateMaintenance: vi.fn(),
  fetchMaintenance: vi.fn(),
}))

const window: MaintenanceWindow = {
  id: 'mw-1',
  title: 'Database upgrade',
  description: '',
  starts_at: '2099-01-01T02:00:00Z',
  ends_at: '2099-01-01T04:00:00Z',
  active: false,
  components: [
    { id: 'comp-api', name: 'API' },
    { id: 'comp-db', name: 'Database' },
  ],
  created_at: '2098-12-01T00:00:00Z',
}

vi.mock('@/services/statusApi', () => ({
  updateMaintenance,
  createMaintenance: vi.fn(),
  deleteMaintenance: vi.fn(),
}))

vi.mock('@/stores/statusAdmin', () => ({
  useStatusAdminStore: () => ({
    components: [
      { id: 'comp-api', display_name: 'API' },
      { id: 'comp-db', display_name: 'Database' },
      { id: 'comp-web', display_name: 'Website' },
    ],
    maintenance: [window],
    maintenanceLoading: false,
    fetchMaintenance,
  }),
}))

vi.mock('@/composables/useConfirm', () => ({ useConfirm: () => vi.fn() }))

describe('StatusMaintenanceManager', () => {
  beforeEach(() => {
    updateMaintenance.mockReset()
    updateMaintenance.mockResolvedValue(window)
  })

  it('keeps the components of the window it edits', async () => {
    const wrapper = mount(StatusMaintenanceManager)
    const edit = wrapper.findAll('button').find((b) => b.text() === 'Edit')
    expect(edit).toBeDefined()
    await edit!.trigger('click')

    const checked = wrapper
      .findAll('label')
      .filter((l) => {
        const box = l.find('input[type="checkbox"]')
        return box.exists() && (box.element as HTMLInputElement).checked
      })
    expect(checked.map((l) => l.text())).toEqual(['API', 'Database'])

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(updateMaintenance).toHaveBeenCalledOnce()
    const [id, data] = updateMaintenance.mock.calls[0]!
    expect(id).toBe('mw-1')
    expect(data.component_ids).toEqual(['comp-api', 'comp-db'])
  })
})
