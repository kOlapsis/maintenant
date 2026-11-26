// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import App from '@/App.vue'
import router from '@/router'
import { useToast } from '@/composables/useToast'

const reload = vi.hoisted(() => vi.fn())

vi.mock('@/router/chunkReload', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/router/chunkReload')>()
  return {
    ...actual,
    reloadOnMissingChunk: (error: unknown, target: string) => actual.reloadOnMissingChunk(error, target, reload),
  }
})

vi.mock('@/router/routes', () => ({
  buildRoutes: () => [
    { path: '/', component: { render: () => null } },
    { path: '/other', component: { render: () => null } },
    {
      path: '/gone',
      component: () => Promise.reject(new TypeError('Failed to fetch dynamically imported module: /assets/Gone-abc.js')),
    },
  ],
}))

vi.mock('@/components/ui/SessionExpiredOverlay.vue', () => ({ default: { render: () => null } }))

const visit = (path: string) => router.push(path).catch(() => {})

describe('router and a route whose code is gone', () => {
  beforeEach(async () => {
    sessionStorage.clear()
    reload.mockReset()
    useToast().toasts.value = []
    await visit('/')
  })

  it('reloads the page once on the route asked for, then stops', async () => {
    await visit('/gone?tab=1')
    expect(reload).toHaveBeenCalledTimes(1)
    expect(reload).toHaveBeenCalledWith('/gone?tab=1')

    await visit('/gone?tab=1')
    expect(reload).toHaveBeenCalledTimes(1)
  })

  it('reloads again after a route loaded in between', async () => {
    await visit('/gone')
    await visit('/other')
    await visit('/gone')

    expect(reload).toHaveBeenCalledTimes(2)
  })

  it('says the page could not be loaded even when no layout is on screen', async () => {
    const wrapper = mount(App, { global: { plugins: [router] }, attachTo: document.body })
    await visit('/gone')
    await visit('/gone')
    await wrapper.vm.$nextTick()

    expect(document.body.textContent).toContain('This page could not be loaded')
    wrapper.unmount()
  })
})
