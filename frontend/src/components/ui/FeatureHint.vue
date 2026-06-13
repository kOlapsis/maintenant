<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Info, ExternalLink, X } from 'lucide-vue-next'

const props = withDefaults(
  defineProps<{
    storageKey: string
    title: string
    docHref?: string
    docLabel?: string
    legacyStorageKey?: string
  }>(),
  {
    docLabel: 'Learn more',
  },
)

const visible = ref(false)

const key = () => `mnt:hint:${props.storageKey}`
const legacyKey = () => `pb:hint:${props.storageKey}`

onMounted(() => {
  if (localStorage.getItem(key()) === '1') return
  // Honour the legacy "pb:" dismissal key so existing users aren't re-prompted.
  if (localStorage.getItem(legacyKey()) === '1') {
    localStorage.setItem(key(), '1')
    return
  }
  if (props.legacyStorageKey && localStorage.getItem(props.legacyStorageKey) === '1') {
    localStorage.setItem(key(), '1')
    return
  }
  visible.value = true
})

function dismiss() {
  visible.value = false
  localStorage.setItem(key(), '1')
  if (props.legacyStorageKey) {
    localStorage.removeItem(props.legacyStorageKey)
  }
}
</script>

<template>
  <div
    v-if="visible"
    class="mb-6 rounded-2xl p-4 bg-mnt-green-500/10 border border-mnt-green-500/20"
  >
    <div class="flex items-start gap-3">
      <Info :size="20" class="text-mnt-green-400 shrink-0 mt-0.5" />
      <div class="flex-1 min-w-0">
        <h3 class="text-sm font-medium text-mnt-green-400">{{ title }}</h3>
        <p class="mt-1 text-sm text-mnt-muted">
          <slot />
          <a
            v-if="docHref"
            :href="docHref"
            target="_blank"
            rel="noopener noreferrer"
            class="ml-1 inline-flex items-center gap-1 text-mnt-green-400 hover:text-mnt-green-300 underline underline-offset-2 decoration-dotted"
          >
            {{ docLabel }}
            <ExternalLink :size="12" class="shrink-0" />
          </a>
        </p>
      </div>
      <button
        @click="dismiss"
        class="text-mnt-muted hover:text-mnt-secondary shrink-0 transition-colors"
        aria-label="Dismiss hint"
      >
        <X :size="16" />
      </button>
    </div>
  </div>
</template>
