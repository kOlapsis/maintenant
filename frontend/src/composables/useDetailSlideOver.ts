// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { ref, readonly, watch, type InjectionKey, type Ref, type DeepReadonly } from 'vue'
import { useRoute, useRouter } from 'vue-router'

export type EntityType = 'container' | 'heartbeat' | 'certificate' | 'endpoint' | 'update' | 'swarm-service' | 'k8s-workload' | 'k8s-pod'

const VALID_ENTITY_TYPES: ReadonlySet<string> = new Set<EntityType>([
  'container',
  'heartbeat',
  'certificate',
  'endpoint',
  'update',
  'swarm-service',
  'k8s-workload',
  'k8s-pod',
])

export interface DetailSlideOver {
  isOpen: DeepReadonly<Ref<boolean>>
  entityType: DeepReadonly<Ref<EntityType | null>>
  entityId: DeepReadonly<Ref<string | null>>
  openDetail: (type: EntityType, id: string) => void
  close: () => void
}

export const detailSlideOverKey: InjectionKey<DetailSlideOver> = Symbol('detailSlideOver')

export function parseSelectedParam(value: unknown): { type: EntityType; id: string } | null {
  if (typeof value !== 'string') return null

  // Handle multi-segment types like 'swarm-service' — find the entity type first
  let type: string | null = null
  let idStr: string | null = null

  for (const candidate of VALID_ENTITY_TYPES) {
    if (value.startsWith(`${candidate}-`)) {
      type = candidate
      idStr = value.slice(candidate.length + 1)
      break
    }
  }

  if (!type || !idStr) return null
  if (!VALID_ENTITY_TYPES.has(type)) return null

  // All entity ids are now opaque strings (UUIDs or composite ids); any
  // non-empty id segment is valid.
  return { type: type as EntityType, id: idStr }
}

export function useDetailSlideOver(): DetailSlideOver {
  const route = useRoute()
  const router = useRouter()

  const isOpen = ref(false)
  const entityType = ref<EntityType | null>(null)
  const entityId = ref<string | null>(null)

  let updatingUrl = false

  function openDetail(type: EntityType, id: string) {
    entityType.value = type
    entityId.value = id
    isOpen.value = true
    syncToUrl(type, id)
  }

  function close() {
    isOpen.value = false
    entityType.value = null
    entityId.value = null
    removeFromUrl()
  }

  function syncToUrl(type: EntityType, id: string) {
    updatingUrl = true
    router.replace({
      query: { ...route.query, selected: `${type}-${id}` },
    }).finally(() => {
      updatingUrl = false
    })
  }

  function removeFromUrl() {
    if (!route.query.selected) return
    updatingUrl = true
    const { selected: _, ...rest } = route.query
    router.replace({ query: rest }).finally(() => {
      updatingUrl = false
    })
  }

  // Close slide-over on route path change (navigation via sidebar)
  watch(
    () => route.path,
    () => {
      if (isOpen.value) {
        isOpen.value = false
        entityType.value = null
        entityId.value = null
      }
    },
  )

  return {
    isOpen: readonly(isOpen),
    entityType: readonly(entityType),
    entityId: readonly(entityId),
    openDetail,
    close,
  }
}
