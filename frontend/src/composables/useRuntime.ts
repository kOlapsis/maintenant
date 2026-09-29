// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

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
