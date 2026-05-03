<script setup lang="ts">
import { ref, computed } from 'vue'
import { Upload, Trash2, X } from 'lucide-vue-next'
import { personalizationApi } from '@/services/personalizationApi'
import { usePersonalizationStore } from '@/stores/personalization'

type AssetRole = 'logo' | 'favicon' | 'hero'

const store = usePersonalizationStore()

const title = defineModel<string>('title', { required: true })
const subtitle = defineModel<string>('subtitle', { required: true })

const logoInputRef = ref<HTMLInputElement | null>(null)
const faviconInputRef = ref<HTMLInputElement | null>(null)
const heroInputRef = ref<HTMLInputElement | null>(null)

const logoFile = ref<File | null>(null)
const faviconFile = ref<File | null>(null)
const heroFile = ref<File | null>(null)

const logoAlt = ref('')
const heroAlt = ref('')

const removeLogo = ref(false)
const removeFavicon = ref(false)
const removeHero = ref(false)

const hasLogo = computed(() => !!store.settings)
const hasFavicon = computed(() => !!store.settings)
const hasHero = computed(() => !!store.settings)

const logoPreviewUrl = computed(() => personalizationApi.getAssetURL('logo'))
const faviconPreviewUrl = computed(() => personalizationApi.getAssetURL('favicon'))
const heroPreviewUrl = computed(() => personalizationApi.getAssetURL('hero'))

function inputRefFor(role: AssetRole): HTMLInputElement | null {
  if (role === 'logo') return logoInputRef.value
  if (role === 'favicon') return faviconInputRef.value
  return heroInputRef.value
}

function onFileSelected(role: AssetRole, e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0] ?? null
  if (role === 'logo') {
    logoFile.value = file
    if (file) removeLogo.value = false
  } else if (role === 'favicon') {
    faviconFile.value = file
    if (file) removeFavicon.value = false
  } else {
    heroFile.value = file
    if (file) removeHero.value = false
  }
}

function clearSelection(role: AssetRole) {
  if (role === 'logo') logoFile.value = null
  else if (role === 'favicon') faviconFile.value = null
  else heroFile.value = null
  const input = inputRefFor(role)
  if (input) input.value = ''
}

function markForRemoval(role: AssetRole) {
  clearSelection(role)
  if (role === 'logo') removeLogo.value = true
  else if (role === 'favicon') removeFavicon.value = true
  else removeHero.value = true
}

function undoRemoval(role: AssetRole) {
  if (role === 'logo') removeLogo.value = false
  else if (role === 'favicon') removeFavicon.value = false
  else removeHero.value = false
}

function pendingFile(role: AssetRole): File | null {
  if (role === 'logo') return logoFile.value
  if (role === 'favicon') return faviconFile.value
  return heroFile.value
}

function pendingRemove(role: AssetRole): boolean {
  if (role === 'logo') return removeLogo.value
  if (role === 'favicon') return removeFavicon.value
  return removeHero.value
}

function pendingAlt(role: AssetRole): string | undefined {
  if (role === 'logo') return logoAlt.value
  if (role === 'hero') return heroAlt.value
  return undefined
}

async function flushPendingAssets(): Promise<void> {
  const roles: AssetRole[] = ['logo', 'favicon', 'hero']
  for (const role of roles) {
    if (pendingRemove(role)) {
      await store.deleteAsset(role)
      undoRemoval(role)
    } else {
      const file = pendingFile(role)
      if (file) {
        await store.uploadAsset(role, file, pendingAlt(role))
        clearSelection(role)
      }
    }
  }
}

defineExpose({ flushPendingAssets })
</script>

<template>
  <div class="space-y-6">
    <h3
      class="text-[10px] font-bold uppercase tracking-widest"
      style="color: var(--pb-text-muted)"
    >
      Branding
    </h3>

    <div class="grid grid-cols-1 gap-4">
      <div>
        <label
          class="block text-xs mb-1"
          style="color: var(--pb-text-muted)"
        >Page Title</label>
        <input
          v-model="title"
          type="text"
          maxlength="100"
          class="w-full rounded-lg border px-3 py-2 text-sm focus:outline-none focus:border-pb-accent"
          style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)"
          placeholder="System Status"
        />
        <p class="mt-1 text-[11px]" style="color: var(--pb-text-muted)">1–100 characters</p>
      </div>

      <div>
        <label
          class="block text-xs mb-1"
          style="color: var(--pb-text-muted)"
        >Subtitle</label>
        <input
          v-model="subtitle"
          type="text"
          maxlength="200"
          class="w-full rounded-lg border px-3 py-2 text-sm focus:outline-none focus:border-pb-accent"
          style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)"
          placeholder="Real-time service health"
        />
        <p class="mt-1 text-[11px]" style="color: var(--pb-text-muted)">0–200 characters</p>
      </div>
    </div>

    <!-- Logo -->
    <div class="space-y-2">
      <label class="block text-xs" style="color: var(--pb-text-muted)">Logo</label>
      <p class="text-[11px]" style="color: var(--pb-text-muted)">
        PNG, JPEG, WebP or SVG — max 200 KB. Recommended: 200×80px
      </p>
      <div class="flex flex-wrap items-center gap-2">
        <input
          ref="logoInputRef"
          type="file"
          accept="image/png,image/jpeg,image/webp,image/svg+xml"
          class="sr-only"
          @change="(e) => onFileSelected('logo', e)"
        />
        <button
          type="button"
          class="inline-flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-xs font-medium transition-colors"
          style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)"
          @click="logoInputRef?.click()"
        >
          <Upload :size="13" />
          Choose file…
        </button>
        <span
          v-if="logoFile"
          class="truncate max-w-[200px] text-xs"
          style="color: var(--pb-text-muted)"
          :title="logoFile.name"
        >
          {{ logoFile.name }}
        </span>
        <button
          v-if="logoFile"
          type="button"
          class="inline-flex items-center gap-1 text-xs hover:underline"
          style="color: var(--pb-text-muted)"
          @click="clearSelection('logo')"
        >
          <X :size="12" />
          Clear
        </button>
        <button
          v-if="hasLogo && !logoFile && !removeLogo"
          type="button"
          class="inline-flex items-center gap-1 rounded-lg border px-3 py-1.5 text-xs font-medium transition-colors hover:border-red-400/40 hover:text-red-400"
          style="background: transparent; border-color: var(--pb-border-default); color: var(--pb-text-muted)"
          @click="markForRemoval('logo')"
        >
          <Trash2 :size="12" />
          Remove
        </button>
        <span
          v-if="removeLogo"
          class="inline-flex items-center gap-2 rounded-lg border px-3 py-1.5 text-xs"
          style="background: rgba(239,68,68,0.08); border-color: rgba(239,68,68,0.3); color: #f87171"
        >
          Will be removed on save
          <button
            type="button"
            class="hover:underline"
            @click="undoRemoval('logo')"
          >
            Undo
          </button>
        </span>
      </div>
      <input
        v-model="logoAlt"
        type="text"
        maxlength="200"
        class="w-full rounded-lg border px-3 py-2 text-sm focus:outline-none focus:border-pb-accent"
        style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)"
        placeholder="Alt text for logo"
      />
    </div>

    <!-- Favicon -->
    <div class="space-y-2">
      <label class="block text-xs" style="color: var(--pb-text-muted)">Favicon</label>
      <p class="text-[11px]" style="color: var(--pb-text-muted)">
        PNG, ICO or SVG — max 50 KB. Recommended: 32×32px
      </p>
      <div class="flex flex-wrap items-center gap-2">
        <input
          ref="faviconInputRef"
          type="file"
          accept="image/png,image/x-icon,image/vnd.microsoft.icon,image/svg+xml"
          class="sr-only"
          @change="(e) => onFileSelected('favicon', e)"
        />
        <button
          type="button"
          class="inline-flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-xs font-medium transition-colors"
          style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)"
          @click="faviconInputRef?.click()"
        >
          <Upload :size="13" />
          Choose file…
        </button>
        <span
          v-if="faviconFile"
          class="truncate max-w-[200px] text-xs"
          style="color: var(--pb-text-muted)"
          :title="faviconFile.name"
        >
          {{ faviconFile.name }}
        </span>
        <button
          v-if="faviconFile"
          type="button"
          class="inline-flex items-center gap-1 text-xs hover:underline"
          style="color: var(--pb-text-muted)"
          @click="clearSelection('favicon')"
        >
          <X :size="12" />
          Clear
        </button>
        <button
          v-if="hasFavicon && !faviconFile && !removeFavicon"
          type="button"
          class="inline-flex items-center gap-1 rounded-lg border px-3 py-1.5 text-xs font-medium transition-colors hover:border-red-400/40 hover:text-red-400"
          style="background: transparent; border-color: var(--pb-border-default); color: var(--pb-text-muted)"
          @click="markForRemoval('favicon')"
        >
          <Trash2 :size="12" />
          Remove
        </button>
        <span
          v-if="removeFavicon"
          class="inline-flex items-center gap-2 rounded-lg border px-3 py-1.5 text-xs"
          style="background: rgba(239,68,68,0.08); border-color: rgba(239,68,68,0.3); color: #f87171"
        >
          Will be removed on save
          <button
            type="button"
            class="hover:underline"
            @click="undoRemoval('favicon')"
          >
            Undo
          </button>
        </span>
      </div>
    </div>

    <!-- Hero -->
    <div class="space-y-2">
      <label class="block text-xs" style="color: var(--pb-text-muted)">Hero Image</label>
      <p class="text-[11px]" style="color: var(--pb-text-muted)">
        PNG, JPEG or WebP — max 500 KB. Recommended: 1200×400px
      </p>
      <div class="flex flex-wrap items-center gap-2">
        <input
          ref="heroInputRef"
          type="file"
          accept="image/png,image/jpeg,image/webp"
          class="sr-only"
          @change="(e) => onFileSelected('hero', e)"
        />
        <button
          type="button"
          class="inline-flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-xs font-medium transition-colors"
          style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)"
          @click="heroInputRef?.click()"
        >
          <Upload :size="13" />
          Choose file…
        </button>
        <span
          v-if="heroFile"
          class="truncate max-w-[200px] text-xs"
          style="color: var(--pb-text-muted)"
          :title="heroFile.name"
        >
          {{ heroFile.name }}
        </span>
        <button
          v-if="heroFile"
          type="button"
          class="inline-flex items-center gap-1 text-xs hover:underline"
          style="color: var(--pb-text-muted)"
          @click="clearSelection('hero')"
        >
          <X :size="12" />
          Clear
        </button>
        <button
          v-if="hasHero && !heroFile && !removeHero"
          type="button"
          class="inline-flex items-center gap-1 rounded-lg border px-3 py-1.5 text-xs font-medium transition-colors hover:border-red-400/40 hover:text-red-400"
          style="background: transparent; border-color: var(--pb-border-default); color: var(--pb-text-muted)"
          @click="markForRemoval('hero')"
        >
          <Trash2 :size="12" />
          Remove
        </button>
        <span
          v-if="removeHero"
          class="inline-flex items-center gap-2 rounded-lg border px-3 py-1.5 text-xs"
          style="background: rgba(239,68,68,0.08); border-color: rgba(239,68,68,0.3); color: #f87171"
        >
          Will be removed on save
          <button
            type="button"
            class="hover:underline"
            @click="undoRemoval('hero')"
          >
            Undo
          </button>
        </span>
      </div>
      <input
        v-model="heroAlt"
        type="text"
        maxlength="200"
        class="w-full rounded-lg border px-3 py-2 text-sm focus:outline-none focus:border-pb-accent"
        style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)"
        placeholder="Alt text for hero image"
      />
    </div>

    <div class="hidden">{{ logoPreviewUrl }} {{ faviconPreviewUrl }} {{ heroPreviewUrl }}</div>
  </div>
</template>
