import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import StatusIncidentManager from '@/commercial/components/status/StatusIncidentManager.vue'

const { createIncident, postIncidentUpdate, deleteIncident, listIncidents, fetchIncidents, confirm } = vi.hoisted(() => ({
  listIncidents: vi.fn(),
  createIncident: vi.fn(),
  postIncidentUpdate: vi.fn(),
  deleteIncident: vi.fn(),
  fetchIncidents: vi.fn(),
  confirm: vi.fn(),
}))

vi.mock('@/services/statusApi', () => ({ createIncident, postIncidentUpdate, deleteIncident, listIncidents }))

vi.mock('@/stores/statusAdmin', () => ({
  useStatusAdminStore: () => ({
    components: [],
    incidents: [
      { id: 'inc-1', title: 'DB down', severity: 'major', status: 'investigating', components: [], updates: [], created_at: '2026-11-01T10:00:00Z' },
    ],
    incidentsTotal: 1,
    incidentsLoading: false,
    fetchIncidents,
  }),
}))

vi.mock('@/composables/useConfirm', () => ({ useConfirm: () => confirm }))

function button(wrapper: ReturnType<typeof mount>, label: string) {
  const b = wrapper.findAll('button').find((x) => x.text() === label)
  if (!b) throw new Error(`button "${label}" is missing`)
  return b
}

async function openCreate() {
  const wrapper = mount(StatusIncidentManager)
  await button(wrapper, 'Create Incident').trigger('click')
  return wrapper
}

describe('StatusIncidentManager failures', () => {
  beforeEach(() => {
    for (const fn of [createIncident, postIncidentUpdate, deleteIncident, listIncidents, fetchIncidents, confirm]) fn.mockReset()
    confirm.mockResolvedValue(true)
  })

  it('shows the error when creating fails and keeps the form open', async () => {
    createIncident.mockRejectedValue(new Error('HTTP 500'))
    const wrapper = await openCreate()
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toContain('HTTP 500')
    expect(wrapper.find('form').exists()).toBe(true)
  })

  it('sends one request for two quick submits', async () => {
    createIncident.mockResolvedValue({})
    const wrapper = await openCreate()
    const form = wrapper.find('form')
    void form.trigger('submit')
    void form.trigger('submit')
    await flushPromises()
    expect(createIncident).toHaveBeenCalledOnce()
  })

  it('shows the error when posting an update fails', async () => {
    postIncidentUpdate.mockRejectedValue(new Error('HTTP 502'))
    const wrapper = mount(StatusIncidentManager)
    await button(wrapper, 'Post Update').trigger('click')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toContain('HTTP 502')
  })

  it('sends one request for two quick update submits', async () => {
    postIncidentUpdate.mockResolvedValue({})
    const wrapper = mount(StatusIncidentManager)
    await button(wrapper, 'Post Update').trigger('click')
    const form = wrapper.find('form')
    void form.trigger('submit')
    void form.trigger('submit')
    await flushPromises()
    expect(postIncidentUpdate).toHaveBeenCalledOnce()
  })

  it('shows the error when deleting fails', async () => {
    deleteIncident.mockRejectedValue(new Error('HTTP 403'))
    const wrapper = mount(StatusIncidentManager)
    await button(wrapper, 'Delete').trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toContain('HTTP 403')
  })

  it('sends one request for two quick deletes', async () => {
    deleteIncident.mockResolvedValue(undefined)
    const wrapper = mount(StatusIncidentManager)
    const del = button(wrapper, 'Delete')
    void del.trigger('click')
    void del.trigger('click')
    await flushPromises()
    expect(deleteIncident).toHaveBeenCalledOnce()
  })

  it('asks the server for the filtered incidents and leaves the shared list alone', async () => {
    listIncidents.mockResolvedValue({
      incidents: [
        { id: 'inc-9', title: 'Old outage', severity: 'major', status: 'resolved', components: [], updates: [], created_at: '2026-10-01T09:00:00Z' },
      ],
      total: 1,
    })
    const wrapper = mount(StatusIncidentManager)
    expect(wrapper.text()).toContain('DB down')

    await wrapper.find('select').setValue('resolved')
    expect(wrapper.text()).not.toContain('DB down')
    expect(wrapper.text()).not.toContain('No incidents')
    await flushPromises()

    expect(listIncidents).toHaveBeenCalledWith({ status: 'resolved' })
    expect(wrapper.text()).toContain('Old outage')
    expect(wrapper.text()).not.toContain('DB down')
    expect(fetchIncidents).not.toHaveBeenCalled()
  })

  it('says when the filtered incidents could not be loaded', async () => {
    listIncidents.mockRejectedValue(new Error('HTTP 503'))
    const wrapper = mount(StatusIncidentManager)

    await wrapper.find('select').setValue('resolved')
    await flushPromises()

    expect(wrapper.find('[role="alert"]').text()).toContain('HTTP 503')
    expect(wrapper.text()).not.toContain('No incidents')
  })
})
