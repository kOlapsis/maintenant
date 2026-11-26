// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

import { showToast } from '@/composables/useToast'

const RELOADED_KEY = 'mnt:chunk-reload'
const MISSING_CHUNK = /dynamically imported module|Importing a module script failed|Unable to preload CSS/i

/** Reloads the page once on the target route when its lazy chunk no longer exists on the server. */
export function reloadOnMissingChunk(
  error: unknown,
  target: string,
  reload: (url: string) => void = (url) => window.location.assign(url),
) {
  if (!(error instanceof Error) || !MISSING_CHUNK.test(error.message)) {
    console.error(error)
    return
  }
  if (!navigator.onLine) {
    showToast('You are offline. Open this page again once the connection is back.', 'warning')
    return
  }
  if (sessionStorage.getItem(RELOADED_KEY)) {
    showToast('This page could not be loaded. Reload the application to get its latest version.', 'warning')
    return
  }
  sessionStorage.setItem(RELOADED_KEY, target)
  reload(target)
}

export function routeLoaded() {
  sessionStorage.removeItem(RELOADED_KEY)
}
