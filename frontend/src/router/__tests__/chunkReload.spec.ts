// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { reloadOnMissingChunk, routeLoaded } from '@/router/chunkReload'

const showToast = vi.hoisted(() => vi.fn())
vi.mock('@/composables/useToast', () => ({ showToast }))

const missing = new TypeError('Failed to fetch dynamically imported module: https://app.example/assets/AlertsPage-abc.js')

describe('reloadOnMissingChunk', () => {
  beforeEach(() => {
    sessionStorage.clear()
    showToast.mockReset()
  })

  it('reloads the page on the route whose chunk is gone', () => {
    const reload = vi.fn()

    reloadOnMissingChunk(missing, '/alerts', reload)

    expect(reload).toHaveBeenCalledWith('/alerts')
  })

  it('reloads once, then says the page could not be loaded', () => {
    const reload = vi.fn()

    reloadOnMissingChunk(missing, '/alerts', reload)
    reloadOnMissingChunk(missing, '/alerts', reload)

    expect(reload).toHaveBeenCalledTimes(1)
    expect(showToast).toHaveBeenCalledTimes(1)
  })

  it('reloads again for a later deployment once a route has loaded', () => {
    const reload = vi.fn()

    reloadOnMissingChunk(missing, '/alerts', reload)
    routeLoaded()
    reloadOnMissingChunk(missing, '/alerts', reload)

    expect(reload).toHaveBeenCalledTimes(2)
  })

  it('only logs any other navigation error', () => {
    const reload = vi.fn()
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {})

    reloadOnMissingChunk(new Error('boom'), '/alerts', reload)

    expect(logged).toHaveBeenCalled()
    expect(reload).not.toHaveBeenCalled()
    expect(showToast).not.toHaveBeenCalled()
  })

  it('keeps the page when the browser is offline, and reloads once it is back', () => {
    const reload = vi.fn()
    const online = vi.spyOn(navigator, 'onLine', 'get').mockReturnValue(false)

    reloadOnMissingChunk(missing, '/alerts', reload)
    expect(reload).not.toHaveBeenCalled()
    expect(showToast).toHaveBeenCalledWith('You are offline. Open this page again once the connection is back.', 'warning')

    online.mockReturnValue(true)
    reloadOnMissingChunk(missing, '/alerts', reload)
    expect(reload).toHaveBeenCalledTimes(1)
    online.mockRestore()
  })
})
