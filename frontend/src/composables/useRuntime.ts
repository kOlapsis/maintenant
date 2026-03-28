// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

import { computed } from 'vue'
import { useRuntimeStore } from '@/stores/runtime'

export function useRuntime() {
  const store = useRuntimeStore()

  return {
    runtimeContext: computed(() => store.context),
    isDocker: computed(() => store.isDocker),
    isSwarm: computed(() => store.isSwarm),
    isKubernetes: computed(() => store.isKubernetes),
    runtimeLabel: computed(() => store.label),
    connected: computed(() => store.connected),
  }
}
