<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)

  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. You may not use this file except in compliance
  with one of these licenses.

  AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
  Commercial: See COMMERCIAL-LICENSE.md

  Source: https://github.com/kolapsis/maintenant
-->

<script setup lang="ts">
import { ref, toRef, onMounted, onUnmounted, computed, watch } from 'vue'
import {
  getContainer,
  listTransitions,
  deleteContainer,
  type ContainerDetailResponse,
  type StateTransition,
} from '@/services/containerApi'
import { useResourcesStore } from '@/stores/resources'
import { timeAgo } from '@/utils/time'
import LogViewer from './LogViewer.vue'
import LogExpandedView from './LogExpandedView.vue'
import { useLogStream } from '@/composables/useLogStream'
import { useLogSearch } from '@/composables/useLogSearch'
import ContainerEventTimeline from './ContainerEventTimeline.vue'
import SecurityInsightList from './SecurityInsightList.vue'
import PostureScoreBadge from './PostureScoreBadge.vue'
import PostureCategoryBreakdown from './PostureCategoryBreakdown.vue'
import { useSecurityStore } from '@/stores/security'
import { usePostureStore } from '@/stores/posture'
import { useEdition } from '@/composables/useEdition'
import type { SecurityScore } from '@/services/postureApi'
import { getStateStyle, getExitCodeStyle } from '@/utils/containerState'
import {
  X,
  Trash2,
  Terminal,
  Activity,
  Clock,
  Server,
  ChevronDown,
  ChevronRight,
} from 'lucide-vue-next'

const props = defineProps<{
  containerId: number
}>()

const emit = defineEmits<{
  close: []
  deleted: [containerId: number]
}>()

const container = ref<ContainerDetailResponse | null>(null)
const transitions = ref<StateTransition[]>([])
const loading = ref(true)
const error = ref<string | null>(null)
const selectedLogContainer = ref<string | undefined>(undefined)
const activeTab = ref<'logs' | 'info'>('info')
const isLogExpanded = ref(false)

const logStream = useLogStream({
  containerId: toRef(props, 'containerId'),
  containerName: selectedLogContainer,
})
const logSearch = useLogSearch(logStream.lines)
const deleting = ref(false)
const confirmingDelete = ref(false)
const resourcesStore = useResourcesStore()
const securityStore = useSecurityStore()
const postureStore = usePostureStore()
const { hasFeature } = useEdition()

const containerPosture = ref<SecurityScore | null>(null)

const containerInsights = computed(() => {
  const ci = securityStore.getContainerInsights(props.containerId)
  return ci?.insights ?? []
})

const containerAcks = computed(() => postureStore.acknowledgments[props.containerId] ?? [])
const showAcknowledge = computed(() => hasFeature('security_posture'))

async function handleAcknowledge(insight: { type: string; details: Record<string, unknown> }) {
  const port = insight.details.port
  const proto = insight.details.protocol || 'tcp'
  const key = port ? `${port}/${proto}` : ''
  await postureStore.acknowledgeRisk({
    container_id: props.containerId,
    finding_type: insight.type,
    finding_key: key,
    acknowledged_by: 'user',
    reason: '',
  })
  await postureStore.fetchAcknowledgments(props.containerId)
  containerPosture.value = await postureStore.fetchContainerScore(props.containerId)
}

async function handleRevoke(ack: { id: number }) {
  await postureStore.revokeAcknowledgment(ack.id)
  await postureStore.fetchAcknowledgments(props.containerId)
  containerPosture.value = await postureStore.fetchContainerScore(props.containerId)
}

const hasMultipleContainers = computed(() => {
  const names = container.value?.container_names
  if (!names) return false
  return names.filter(n => !n.endsWith(' (init)')).length > 1
})

const stateConfig = {
  get(state: string) {
    return getStateStyle(state)
  },
}

const metrics = computed(() => {
  if (!container.value) return null
  return resourcesStore.formattedSnapshot(container.value.id)
})

const cpuPercent = computed(() => {
  if (!container.value) return 0
  const snap = resourcesStore.getSnapshot(container.value.id)
  return snap ? Math.min(snap.cpu_percent, 100) : 0
})

const memPercent = computed(() => {
  if (!container.value) return 0
  const snap = resourcesStore.getSnapshot(container.value.id)
  if (!snap || snap.mem_limit === 0) return 0
  return Math.min((snap.mem_used / snap.mem_limit) * 100, 100)
})

function barColor(value: number): string {
  if (value > 90) return 'var(--pb-status-down)'
  if (value > 70) return 'var(--pb-status-warn)'
  return 'var(--pb-status-ok)'
}

function formatTimestamp(iso: string): string {
  return new Date(iso).toLocaleString()
}

const formatRelative = timeAgo

async function loadData() {
  loading.value = true
  error.value = null
  try {
    const [c, t] = await Promise.all([
      getContainer(props.containerId),
      listTransitions(props.containerId, { limit: 20 }),
      securityStore.fetchForContainer(props.containerId),
    ])
    container.value = c
    transitions.value = t.transitions || []
    if (hasFeature('security_posture')) {
      containerPosture.value = await postureStore.fetchContainerScore(props.containerId)
      await postureStore.fetchAcknowledgments(props.containerId)
    }
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load'
  } finally {
    loading.value = false
  }
}

async function handleDelete() {
  if (!confirmingDelete.value) {
    confirmingDelete.value = true
    return
  }
  deleting.value = true
  try {
    await deleteContainer(props.containerId)
    emit('deleted', props.containerId)
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to delete'
  } finally {
    deleting.value = false
    confirmingDelete.value = false
  }
}

function onCtrlK(e: KeyboardEvent) {
  if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
    e.preventDefault()
    logSearch.open()
  }
}

onMounted(() => {
  loadData()
  logStream.connect()
  document.addEventListener('keydown', onCtrlK, true)
})

onUnmounted(() => {
  logStream.disconnect()
  document.removeEventListener('keydown', onCtrlK, true)
})

watch(() => props.containerId, () => {
  selectedLogContainer.value = undefined
  activeTab.value = 'info'
  isLogExpanded.value = false
  loadData()
})
</script>

<template>
  <div class="flex h-full flex-col">
    <!-- Loading -->
    <div v-if="loading" class="flex flex-1 items-center justify-center">
      <div
        class="h-7 w-7 animate-spin rounded-full border-2"
        :style="{ borderColor: 'var(--pb-border-default)', borderTopColor: 'var(--pb-accent)' }"
      />
    </div>

    <!-- Error -->
    <div
      v-else-if="error"
      class="m-4 rounded-lg p-4 text-sm"
      :style="{
        backgroundColor: 'var(--pb-status-down-bg)',
        color: 'var(--pb-status-down)',
      }"
    >
      {{ error }}
    </div>

    <!-- Content -->
    <template v-else-if="container">
      <!-- Compact header strip -->
      <div
        class="flex items-center gap-3 border-b px-5 py-3"
        :style="{ borderColor: 'var(--pb-border-default)' }"
      >
        <!-- State dot -->
        <div
          class="h-3 w-3 shrink-0 rounded-full"
          :style="{
            backgroundColor: stateConfig.get(container.state).color,
            boxShadow: stateConfig.get(container.state).glow || 'none',
          }"
        />
        <!-- Name + image -->
        <div class="min-w-0 flex-1">
          <h2 class="truncate text-sm font-bold" :style="{ color: 'var(--pb-text-primary)' }">
            {{ container.name }}
          </h2>
          <p class="truncate text-xs" :style="{ color: 'var(--pb-text-muted)' }">
            {{ container.image.split('@')[0] }}
          </p>
        </div>
        <!-- State badge -->
        <span
          class="shrink-0 rounded-full px-2.5 py-1 text-xs font-semibold"
          :style="{
            backgroundColor: stateConfig.get(container.state).bg,
            color: stateConfig.get(container.state).color,
          }"
        >
          {{ container.state }}
        </span>
        <!-- Delete button (non-running only) -->
        <button
          v-if="container.state !== 'running'"
          class="shrink-0 rounded-lg p-1.5 transition-colors"
          :style="{
            color: confirmingDelete ? 'var(--pb-status-down)' : 'var(--pb-text-muted)',
            backgroundColor: confirmingDelete ? 'var(--pb-status-down-bg)' : 'transparent',
          }"
          :title="confirmingDelete ? 'Click again to confirm deletion' : 'Remove from database'"
          :disabled="deleting"
          @click="handleDelete"
        >
          <Trash2 :size="14" />
        </button>
      </div>

      <!-- Resource bar (running only) -->
      <div
        v-if="container.state === 'running' && metrics"
        class="flex items-center gap-4 border-b px-5 py-2.5"
        :style="{ borderColor: 'var(--pb-border-default)', backgroundColor: 'var(--pb-bg-primary)' }"
      >
        <!-- CPU -->
        <div class="flex items-center gap-2 text-xs" style="min-width: 140px">
          <span :style="{ color: 'var(--pb-text-muted)' }">CPU</span>
          <div class="h-1.5 flex-1 rounded-full" :style="{ backgroundColor: 'var(--pb-bg-elevated)' }">
            <div
              class="h-1.5 rounded-full transition-all duration-500"
              :style="{ width: cpuPercent + '%', backgroundColor: barColor(cpuPercent) }"
            />
          </div>
          <span class="tabular-nums font-medium" :style="{ color: 'var(--pb-text-secondary)' }">{{ metrics.cpu }}</span>
        </div>
        <!-- MEM -->
        <div class="flex items-center gap-2 text-xs" style="min-width: 140px">
          <span :style="{ color: 'var(--pb-text-muted)' }">MEM</span>
          <div class="h-1.5 flex-1 rounded-full" :style="{ backgroundColor: 'var(--pb-bg-elevated)' }">
            <div
              class="h-1.5 rounded-full transition-all duration-500"
              :style="{ width: memPercent + '%', backgroundColor: barColor(memPercent) }"
            />
          </div>
          <span class="tabular-nums font-medium" :style="{ color: 'var(--pb-text-secondary)' }">{{ metrics.memUsed }}<template v-if="metrics.memLimit !== '0 B'"> / {{ metrics.memLimit }}</template></span>
          <span class="tabular-nums" :style="{ color: 'var(--pb-text-muted)' }">{{ metrics.memPercent }}</span>
        </div>
        <!-- Net/IO -->
        <div class="ml-auto flex gap-3 text-xs" :style="{ color: 'var(--pb-text-muted)' }">
          <span>Net: {{ metrics.netRx }}/{{ metrics.netTx }}</span>
          <span>I/O: {{ metrics.blockRead }}/{{ metrics.blockWrite }}</span>
        </div>
      </div>

      <!-- Tab bar -->
      <div
        class="flex items-center gap-1 border-b px-5"
        :style="{ borderColor: 'var(--pb-border-default)' }"
      >
        <button
          class="flex items-center gap-1.5 px-3 py-2.5 text-xs font-semibold transition-colors"
          :style="{
            color: activeTab === 'info' ? 'var(--pb-accent)' : 'var(--pb-text-muted)',
            borderBottom: activeTab === 'info' ? '2px solid var(--pb-accent)' : '2px solid transparent',
          }"
          @click="activeTab = 'info'"
        >
          <Activity :size="13" />
          Details
        </button>
        <button
          class="flex items-center gap-1.5 px-3 py-2.5 text-xs font-semibold transition-colors"
          :style="{
            color: activeTab === 'logs' ? 'var(--pb-accent)' : 'var(--pb-text-muted)',
            borderBottom: activeTab === 'logs' ? '2px solid var(--pb-accent)' : '2px solid transparent',
          }"
          @click="activeTab = 'logs'"
        >
          <Terminal :size="13" />
          Logs
        </button>

        <!-- K8s container selector (logs tab only) -->
        <select
          v-if="activeTab === 'logs' && hasMultipleContainers && container?.container_names"
          class="ml-auto text-xs"
          :style="{
            backgroundColor: 'var(--pb-bg-elevated)',
            color: 'var(--pb-text-secondary)',
            padding: '0.25rem 0.5rem',
            borderRadius: 'var(--pb-radius-sm)',
            border: '1px solid var(--pb-border-default)',
          }"
          :value="selectedLogContainer || ''"
          @change="selectedLogContainer = ($event.target as HTMLSelectElement).value || undefined"
        >
          <option value="">All containers</option>
          <option
            v-for="name in container.container_names"
            :key="name"
            :value="name.replace(' (init)', '')"
          >{{ name }}</option>
        </select>
      </div>

      <!-- Tab content -->
      <div class="min-h-0 flex-1" :class="activeTab === 'logs' ? 'flex flex-col overflow-hidden' : 'overflow-y-auto'">
        <!-- LOGS TAB -->
        <div v-if="activeTab === 'logs'" class="flex min-h-0 flex-1 flex-col">
          <LogViewer
            :log-stream="logStream"
            :is-expanded="isLogExpanded"
            :search="logSearch"
            class="flex-1"
            @toggle-expand="isLogExpanded = true"
          />
        </div>

        <!-- INFO TAB -->
        <div v-else class="space-y-5 p-5">
          <!-- Info grid -->
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-3 text-sm">
            <div>
              <span class="text-xs font-medium" :style="{ color: 'var(--pb-text-muted)' }">External ID</span>
              <p class="mt-0.5 font-mono text-xs" :style="{ color: 'var(--pb-text-primary)' }">
                {{ container.external_id.slice(0, 12) }}
              </p>
            </div>
            <div>
              <span class="text-xs font-medium" :style="{ color: 'var(--pb-text-muted)' }">First Seen</span>
              <p class="mt-0.5" :style="{ color: 'var(--pb-text-primary)' }">
                {{ formatTimestamp(container.first_seen_at) }}
              </p>
            </div>
            <div>
              <span class="text-xs font-medium" :style="{ color: 'var(--pb-text-muted)' }">Health</span>
              <p class="mt-0.5 font-medium" :style="{ color: 'var(--pb-text-primary)' }">
                <template v-if="container.has_health_check">
                  {{ container.health_status || 'N/A' }}
                </template>
                <span v-else :style="{ color: 'var(--pb-text-muted)', fontStyle: 'italic' }">
                  No health check
                </span>
              </p>
            </div>
            <div v-if="container.orchestration_group">
              <span class="text-xs font-medium" :style="{ color: 'var(--pb-text-muted)' }">Group</span>
              <p class="mt-0.5" :style="{ color: 'var(--pb-text-primary)' }">{{ container.orchestration_group }}</p>
            </div>
            <div v-if="container.orchestration_unit">
              <span class="text-xs font-medium" :style="{ color: 'var(--pb-text-muted)' }">Unit</span>
              <p class="mt-0.5" :style="{ color: 'var(--pb-text-primary)' }">{{ container.orchestration_unit }}</p>
            </div>
            <div v-if="container.error_detail">
              <span class="text-xs font-medium" :style="{ color: 'var(--pb-text-muted)' }">Error</span>
              <p class="mt-0.5 text-xs" :style="{ color: 'var(--pb-status-down)' }">{{ container.error_detail }}</p>
            </div>
            <div v-if="container.runtime_type === 'kubernetes' && container.namespace">
              <span class="text-xs font-medium" :style="{ color: 'var(--pb-text-muted)' }">Namespace</span>
              <p class="mt-0.5" :style="{ color: 'var(--pb-text-primary)' }">{{ container.namespace }}</p>
            </div>
            <div v-if="container.controller_kind">
              <span class="text-xs font-medium" :style="{ color: 'var(--pb-text-muted)' }">Controller</span>
              <p class="mt-0.5" :style="{ color: 'var(--pb-text-primary)' }">{{ container.controller_kind }}</p>
            </div>
            <div v-if="container.runtime_type === 'kubernetes' && container.pod_count">
              <span class="text-xs font-medium" :style="{ color: 'var(--pb-text-muted)' }">Pods</span>
              <p class="mt-0.5" :style="{
                color: container.ready_count === container.pod_count ? 'var(--pb-status-ok)' : 'var(--pb-status-warn)'
              }">{{ container.ready_count }}/{{ container.pod_count }} ready</p>
            </div>
          </div>

          <!-- Security Insights -->
          <SecurityInsightList
            :insights="containerInsights"
            :acknowledgments="containerAcks"
            :show-acknowledge="showAcknowledge"
            @acknowledge="handleAcknowledge"
            @revoke="handleRevoke"
          />

          <!-- Security Posture Score -->
          <div v-if="containerPosture">
            <div class="mb-3 flex items-center gap-3">
              <h3 class="text-xs font-bold uppercase tracking-wider" :style="{ color: 'var(--pb-text-muted)' }">
                Security Score
              </h3>
              <PostureScoreBadge :score="containerPosture.score" :color="containerPosture.color" size="sm" />
            </div>
            <PostureCategoryBreakdown :categories="containerPosture.categories" />
          </div>

          <!-- Event Timeline -->
          <ContainerEventTimeline :transitions="transitions" :hours="24" :current-state="container.state" />

          <!-- State transitions history -->
          <div>
            <h3 class="mb-3 text-xs font-bold uppercase tracking-wider" :style="{ color: 'var(--pb-text-muted)' }">
              State History
            </h3>
            <div v-if="transitions.length === 0" class="text-sm" :style="{ color: 'var(--pb-text-muted)' }">
              No state transitions recorded.
            </div>
            <div v-else class="space-y-1.5">
              <div
                v-for="t in transitions"
                :key="t.id"
                class="flex items-center gap-3 rounded-lg px-3 py-2 text-xs"
                :style="{
                  backgroundColor: 'var(--pb-bg-elevated)',
                  border: '1px solid var(--pb-border-subtle)',
                }"
              >
                <div class="flex items-center gap-1.5 min-w-0 flex-1">
                  <span
                    class="font-medium"
                    :style="{ color: stateConfig.get(t.previous_state).color }"
                  >{{ t.previous_state }}</span>
                  <ChevronRight :size="11" :style="{ color: 'var(--pb-text-muted)' }" />
                  <span
                    class="font-medium"
                    :style="{ color: stateConfig.get(t.new_state).color }"
                  >{{ t.new_state }}</span>
                  <span
                    v-if="t.exit_code !== undefined && t.exit_code !== null"
                    class="ml-1 rounded px-1.5 py-0.5"
                    :style="{
                      ...getExitCodeStyle(t.exit_code),
                      fontSize: '0.65rem',
                    }"
                  >exit {{ t.exit_code }}</span>
                </div>
                <span class="shrink-0 tabular-nums" :style="{ color: 'var(--pb-text-muted)' }">
                  {{ formatRelative(t.timestamp) }} ago
                </span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Expanded log overlay -->
      <LogExpandedView
        v-if="isLogExpanded"
        :container-name="container?.name ?? ''"
        :log-stream="logStream"
        :search="logSearch"
        @close="isLogExpanded = false"
      />
    </template>
  </div>
</template>
