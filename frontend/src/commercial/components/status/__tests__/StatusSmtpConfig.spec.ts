import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import StatusSmtpConfig from '@/commercial/components/status/StatusSmtpConfig.vue'

const { testSmtp } = vi.hoisted(() => ({ testSmtp: vi.fn() }))

vi.mock('@/services/statusApi', () => ({ testSmtp }))

async function sendTestTo(address: string) {
  const wrapper = mount(StatusSmtpConfig)
  await wrapper.find('input[type="email"]').setValue(address)
  await wrapper.find('form').trigger('submit')
  await flushPromises()
  return wrapper
}

describe('StatusSmtpConfig', () => {
  beforeEach(() => {
    testSmtp.mockReset()
  })

  it('shows the environment configuration instead of an editable form', () => {
    const wrapper = mount(StatusSmtpConfig)
    expect(wrapper.text()).toContain('MAINTENANT_SMTP_*')
    expect(wrapper.find('input[type="password"]').exists()).toBe(false)
    expect(wrapper.findAll('input')).toHaveLength(1)
  })

  it('sends the test email to the address typed in', async () => {
    testSmtp.mockResolvedValue({ status: 'sent' })
    const wrapper = await sendTestTo('ops@example.com')

    expect(testSmtp).toHaveBeenCalledWith('ops@example.com')
    expect(wrapper.text()).toContain('Test email sent to ops@example.com')
  })

  it('reports why the server refused the test', async () => {
    testSmtp.mockRejectedValue(new Error('535 authentication failed'))
    const wrapper = await sendTestTo('ops@example.com')

    expect(wrapper.text()).toContain('535 authentication failed')
  })
})
