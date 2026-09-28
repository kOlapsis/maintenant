<script setup lang="ts">
import { ref, computed } from 'vue'
import { Upload, Trash2, X } from 'lucide-vue-next'
import { personalizationApi } from '@/services/personalizationApi'
import { usePersonalizationStore } from '@/stores/personalization'
import FormField from '@/components/ui/FormField.vue'
import TextInput from '@/components/ui/TextInput.vue'
import UiButton from '@/components/ui/UiButton.vue'

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
      style="color: var(--mnt-text-muted)"
    >
      Branding
    </h3>

    <div class="grid grid-cols-1 gap-4">
      <FormField label="Page Title" hint="1–100 characters">
        <template #default="{ id, describedBy, invalid }">
          <TextInput :id="id" v-model="title" maxlength="100" :aria-describedby="describedBy" :invalid="invalid" placeholder="System Status" />
        </template>
      </FormField>

      <FormField label="Subtitle" hint="0–200 characters">
        <template #default="{ id, describedBy, invalid }">
          <TextInput :id="id" v-model="subtitle" maxlength="200" :aria-describedby="describedBy" :invalid="invalid" placeholder="Real-time service health" />
        </template>
      </FormField>
    </div>

    <!-- Logo -->
    <div class="space-y-2">
      <label class="block text-xs" style="color: var(--mnt-text-muted)">Logo</label>
      <p class="text-[11px]" style="color: var(--mnt-text-muted)">
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
        <UiButton type="button" variant="secondary" size="sm" :icon="Upload" @click="logoInputRef?.click()">
          Choose file…
        </UiButton>
        <span
          v-if="logoFile"
          class="truncate max-w-[200px] text-xs"
          style="color: var(--mnt-text-muted)"
          :title="logoFile.name"
        >
          {{ logoFile.name }}
        </span>
        <UiButton v-if="logoFile" type="button" variant="ghost" size="sm" :icon="X" @click="clearSelection('logo')">
          Clear
        </UiButton>
        <UiButton
          v-if="hasLogo && !logoFile && !removeLogo"
          type="button"
          variant="secondary"
          size="sm"
          :icon="Trash2"
          @click="markForRemoval('logo')"
        >
          Remove
        </UiButton>
        <span
          v-if="removeLogo"
          class="inline-flex items-center gap-2 rounded-lg border px-3 py-1.5 text-xs"
          style="background: var(--mnt-status-down-bg); border-color: var(--mnt-status-down); color: var(--mnt-status-down)"
        >
          Will be removed on save
          <UiButton type="button" variant="ghost" size="sm" @click="undoRemoval('logo')">Undo</UiButton>
        </span>
      </div>
      <TextInput v-model="logoAlt" maxlength="200" aria-label="Alt text for logo" placeholder="Alt text for logo" />
    </div>

    <!-- Favicon -->
    <div class="space-y-2">
      <label class="block text-xs" style="color: var(--mnt-text-muted)">Favicon</label>
      <p class="text-[11px]" style="color: var(--mnt-text-muted)">
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
        <UiButton type="button" variant="secondary" size="sm" :icon="Upload" @click="faviconInputRef?.click()">
          Choose file…
        </UiButton>
        <span
          v-if="faviconFile"
          class="truncate max-w-[200px] text-xs"
          style="color: var(--mnt-text-muted)"
          :title="faviconFile.name"
        >
          {{ faviconFile.name }}
        </span>
        <UiButton v-if="faviconFile" type="button" variant="ghost" size="sm" :icon="X" @click="clearSelection('favicon')">
          Clear
        </UiButton>
        <UiButton
          v-if="hasFavicon && !faviconFile && !removeFavicon"
          type="button"
          variant="secondary"
          size="sm"
          :icon="Trash2"
          @click="markForRemoval('favicon')"
        >
          Remove
        </UiButton>
        <span
          v-if="removeFavicon"
          class="inline-flex items-center gap-2 rounded-lg border px-3 py-1.5 text-xs"
          style="background: var(--mnt-status-down-bg); border-color: var(--mnt-status-down); color: var(--mnt-status-down)"
        >
          Will be removed on save
          <UiButton type="button" variant="ghost" size="sm" @click="undoRemoval('favicon')">Undo</UiButton>
        </span>
      </div>
    </div>

    <!-- Hero -->
    <div class="space-y-2">
      <label class="block text-xs" style="color: var(--mnt-text-muted)">Hero Image</label>
      <p class="text-[11px]" style="color: var(--mnt-text-muted)">
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
        <UiButton type="button" variant="secondary" size="sm" :icon="Upload" @click="heroInputRef?.click()">
          Choose file…
        </UiButton>
        <span
          v-if="heroFile"
          class="truncate max-w-[200px] text-xs"
          style="color: var(--mnt-text-muted)"
          :title="heroFile.name"
        >
          {{ heroFile.name }}
        </span>
        <UiButton v-if="heroFile" type="button" variant="ghost" size="sm" :icon="X" @click="clearSelection('hero')">
          Clear
        </UiButton>
        <UiButton
          v-if="hasHero && !heroFile && !removeHero"
          type="button"
          variant="secondary"
          size="sm"
          :icon="Trash2"
          @click="markForRemoval('hero')"
        >
          Remove
        </UiButton>
        <span
          v-if="removeHero"
          class="inline-flex items-center gap-2 rounded-lg border px-3 py-1.5 text-xs"
          style="background: var(--mnt-status-down-bg); border-color: var(--mnt-status-down); color: var(--mnt-status-down)"
        >
          Will be removed on save
          <UiButton type="button" variant="ghost" size="sm" @click="undoRemoval('hero')">Undo</UiButton>
        </span>
      </div>
      <TextInput v-model="heroAlt" maxlength="200" aria-label="Alt text for hero image" placeholder="Alt text for hero image" />
    </div>

    <div class="hidden">{{ logoPreviewUrl }} {{ faviconPreviewUrl }} {{ heroPreviewUrl }}</div>
  </div>
</template>
