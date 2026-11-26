// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ResourceAlertConfig from '@/components/ResourceAlertConfig.vue'

const api = vi.hoisted(() => ({ getAlertConfig: vi.fn(), updateAlertConfig: vi.fn() }))
vi.mock('@/services/resourceApi', () => api)

const saved = { container_id: 'c1', cpu_threshold: 40, mem_threshold: 50, enabled: true, alert_state: 'normal' }

async function mountForm() {
  const wrapper = mount(ResourceAlertConfig, { props: { containerId: 'c1' } })
  await flushPromises()
  return wrapper
}

function saveButton(wrapper: Awaited<ReturnType<typeof mountForm>>) {
  return wrapper.findAll('button').find((b) => b.text() === 'Save')!
}

describe('ResourceAlertConfig', () => {
  beforeEach(() => {
    for (const fn of Object.values(api)) fn.mockReset()
  })

  it('does not let defaults overwrite a configuration it could not read', async () => {
    api.getAlertConfig.mockRejectedValue(new Error('HTTP 503'))
    const wrapper = await mountForm()

    expect(wrapper.find('[role="alert"]').text()).toContain('HTTP 503')
    expect(saveButton(wrapper).attributes('disabled')).toBeDefined()

    await saveButton(wrapper).trigger('click')
    expect(api.updateAlertConfig).not.toHaveBeenCalled()
  })

  it('saves what it loaded', async () => {
    api.getAlertConfig.mockResolvedValue(saved)
    api.updateAlertConfig.mockResolvedValue(saved)
    const wrapper = await mountForm()

    await saveButton(wrapper).trigger('click')

    expect(api.updateAlertConfig).toHaveBeenCalledWith('c1', { cpu_threshold: 40, mem_threshold: 50, enabled: true })
  })
})
