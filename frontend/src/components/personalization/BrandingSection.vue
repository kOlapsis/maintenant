<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { Trash2 } from 'lucide-vue-next'
import { personalizationApi } from '@/services/personalizationApi'
import { usePersonalizationStore } from '@/stores/personalization'
import FormField from '@/components/ui/FormField.vue'
import TextInput from '@/components/ui/TextInput.vue'
import UiButton from '@/components/ui/UiButton.vue'
import FileInput from '@/components/ui/FileInput.vue'

type AssetRole = 'logo' | 'favicon' | 'hero'

const store = usePersonalizationStore()

const title = defineModel<string>('title', { required: true })
const subtitle = defineModel<string>('subtitle', { required: true })

const logoFile = ref<File | null>(null)
const faviconFile = ref<File | null>(null)
const heroFile = ref<File | null>(null)

const logoAlt = ref('')
const heroAlt = ref('')

const removeLogo = ref(false)
const removeFavicon = ref(false)
const removeHero = ref(false)

watch(logoFile, (f) => { if (f) removeLogo.value = false })
watch(faviconFile, (f) => { if (f) removeFavicon.value = false })
watch(heroFile, (f) => { if (f) removeHero.value = false })

const hasLogo = computed(() => !!store.settings)
const hasFavicon = computed(() => !!store.settings)
const hasHero = computed(() => !!store.settings)

const logoPreviewUrl = computed(() => personalizationApi.getAssetURL('logo'))
const faviconPreviewUrl = computed(() => personalizationApi.getAssetURL('favicon'))
const heroPreviewUrl = computed(() => personalizationApi.getAssetURL('hero'))

function clearSelection(role: AssetRole) {
  if (role === 'logo') logoFile.value = null
  else if (role === 'favicon') faviconFile.value = null
  else heroFile.value = null
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
        <FileInput v-model:file="logoFile" accept="image/png,image/jpeg,image/webp,image/svg+xml" />
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
        <FileInput v-model:file="faviconFile" accept="image/png,image/x-icon,image/vnd.microsoft.icon,image/svg+xml" />
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
        <FileInput v-model:file="heroFile" accept="image/png,image/jpeg,image/webp" />
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
