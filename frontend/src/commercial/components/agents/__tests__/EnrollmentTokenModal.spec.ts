import { describe, it, expect, afterEach } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'
import EnrollmentTokenModal from '@/commercial/components/agents/EnrollmentTokenModal.vue'
import type { EnrollmentTokenCreated } from '@/services/agentApi'

const token: EnrollmentTokenCreated = {
  token_id: 'tok-1',
  token_masked: 'mnt_enr_abc...***',
  created_at: '2026-09-30T00:00:00Z',
  expires_at: '2026-10-01T00:00:00Z',
  consumed_at: null,
  consumed_by_agent_id: null,
  token: 'mnt_enr_abc',
  install_templates: {
    standalone: 'curl -fsSL https://install.maintenant.dev | sudo bash -s -- --mode agent',
    docker_run: 'docker run -d',
    docker_compose: 'services:',
    kubernetes: 'apiVersion: v1',
  },
}

let wrapper: VueWrapper | undefined

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  window.localStorage.clear()
})

function buttonLabelled(label: string): HTMLButtonElement | undefined {
  return Array.from(document.querySelectorAll<HTMLButtonElement>('button')).find(
    (b) => b.textContent?.trim() === label,
  )
}

describe('EnrollmentTokenModal', () => {
  it('hands out the standalone installer as a command to copy', async () => {
    wrapper = mount(EnrollmentTokenModal, { props: { token }, attachTo: document.body })

    const standalone = buttonLabelled('Standalone')
    expect(standalone).toBeDefined()
    standalone?.click()
    await nextTick()

    expect(document.querySelector('pre')?.textContent).toBe(token.install_templates.standalone)
    expect(buttonLabelled('Copy install command')).toBeDefined()
  })
})
