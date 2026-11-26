// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { createRouter, createWebHistory } from 'vue-router'
import { buildRoutes } from './routes'
import { reloadOnMissingChunk, routeLoaded } from './chunkReload'

const isStatusSubdomain =
  document.querySelector('meta[name="maintenant-status"]')?.getAttribute('content') === 'true' ||
  // Fallback for an index.html cached by the service worker before the meta tag shipped.
  (window as unknown as { __MAINTENANT_STATUS?: boolean }).__MAINTENANT_STATUS === true

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: buildRoutes(isStatusSubdomain),
})

router.onError((error, to) => reloadOnMissingChunk(error, to.fullPath))
router.afterEach((_to, _from, failure) => {
  if (!failure) routeLoaded()
})

export default router
