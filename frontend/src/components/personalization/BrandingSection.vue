<script setup lang="ts">
import { ref, computed } from 'vue'
import { personalizationApi } from '@/services/personalizationApi'
import { usePersonalizationStore } from '@/stores/personalization'

const store = usePersonalizationStore()

const title = defineModel<string>('title', { required: true })
const subtitle = defineModel<string>('subtitle', { required: true })

const logoFile = ref<File | null>(null)
const logoAlt = ref(store.settings?.announcement?.message_md ?? '')
const faviconFile = ref<File | null>(null)
const heroFile = ref<File | null>(null)
const heroAlt = ref('')
const uploadError = ref('')
const uploading = ref(false)

const logoPreviewUrl = computed(() => personalizationApi.getAssetURL('logo'))
const faviconPreviewUrl = computed(() => personalizationApi.getAssetURL('favicon'))
const heroPreviewUrl = computed(() => personalizationApi.getAssetURL('hero'))

const hasLogo = computed(() => !!store.settings)
const hasFavicon = computed(() => !!store.settings)
const hasHero = computed(() => !!store.settings)

async function uploadAsset(role: string, file: File | null, altText?: string) {
  if (!file) return
  uploading.value = true
  uploadError.value = ''
  try {
    await store.uploadAsset(role, file, altText)
  } catch (e) {
    uploadError.value = e instanceof Error ? e.message : 'Upload failed'
  } finally {
    uploading.value = false
  }
}

async function deleteAsset(role: string) {
  await store.deleteAsset(role)
}
</script>

<template>
  <div class="space-y-6">
    <h3 class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">Branding</h3>

    <div class="grid grid-cols-1 gap-4">
      <div>
        <label class="block text-xs text-slate-400 mb-1">Page Title</label>
        <input
          v-model="title"
          type="text"
          maxlength="100"
          class="w-full bg-[#0B0E13] border border-slate-800 rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-slate-600"
          placeholder="System Status"
        />
        <p class="text-[11px] text-slate-600 mt-1">1–100 characters</p>
      </div>

      <div>
        <label class="block text-xs text-slate-400 mb-1">Subtitle</label>
        <input
          v-model="subtitle"
          type="text"
          maxlength="200"
          class="w-full bg-[#0B0E13] border border-slate-800 rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-slate-600"
          placeholder="Real-time service health"
        />
        <p class="text-[11px] text-slate-600 mt-1">0–200 characters</p>
      </div>
    </div>

    <!-- Logo -->
    <div class="space-y-2">
      <label class="block text-xs text-slate-400">Logo</label>
      <p class="text-[11px] text-slate-600">PNG, JPEG, WebP or SVG — max 200 KB. Recommended: 200×80px</p>
      <div class="flex items-center gap-3">
        <input
          type="file"
          accept="image/png,image/jpeg,image/webp,image/svg+xml"
          class="text-xs text-slate-400"
          @change="(e: Event) => logoFile = (e.target as HTMLInputElement).files?.[0] ?? null"
        />
        <button
          v-if="logoFile"
          class="text-xs px-3 py-1 bg-pb-green-600 rounded text-white hover:bg-pb-green-500"
          :disabled="uploading"
          @click="uploadAsset('logo', logoFile, logoAlt)"
        >
          Upload
        </button>
        <button
          v-if="hasLogo"
          class="text-xs px-3 py-1 bg-slate-800 rounded text-slate-400 hover:text-red-400"
          @click="deleteAsset('logo')"
        >
          Remove
        </button>
      </div>
      <input
        v-model="logoAlt"
        type="text"
        maxlength="200"
        class="w-full bg-[#0B0E13] border border-slate-800 rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-slate-600"
        placeholder="Alt text for logo"
      />
    </div>

    <!-- Favicon -->
    <div class="space-y-2">
      <label class="block text-xs text-slate-400">Favicon</label>
      <p class="text-[11px] text-slate-600">PNG, ICO or SVG — max 50 KB. Recommended: 32×32px</p>
      <div class="flex items-center gap-3">
        <input
          type="file"
          accept="image/png,image/x-icon,image/vnd.microsoft.icon,image/svg+xml"
          class="text-xs text-slate-400"
          @change="(e: Event) => faviconFile = (e.target as HTMLInputElement).files?.[0] ?? null"
        />
        <button
          v-if="faviconFile"
          class="text-xs px-3 py-1 bg-pb-green-600 rounded text-white hover:bg-pb-green-500"
          :disabled="uploading"
          @click="uploadAsset('favicon', faviconFile)"
        >
          Upload
        </button>
        <button
          v-if="hasFavicon"
          class="text-xs px-3 py-1 bg-slate-800 rounded text-slate-400 hover:text-red-400"
          @click="deleteAsset('favicon')"
        >
          Remove
        </button>
      </div>
    </div>

    <!-- Hero -->
    <div class="space-y-2">
      <label class="block text-xs text-slate-400">Hero Image</label>
      <p class="text-[11px] text-slate-600">PNG, JPEG or WebP — max 500 KB. Recommended: 1200×400px</p>
      <div class="flex items-center gap-3">
        <input
          type="file"
          accept="image/png,image/jpeg,image/webp"
          class="text-xs text-slate-400"
          @change="(e: Event) => heroFile = (e.target as HTMLInputElement).files?.[0] ?? null"
        />
        <button
          v-if="heroFile"
          class="text-xs px-3 py-1 bg-pb-green-600 rounded text-white hover:bg-pb-green-500"
          :disabled="uploading"
          @click="uploadAsset('hero', heroFile, heroAlt)"
        >
          Upload
        </button>
        <button
          v-if="hasHero"
          class="text-xs px-3 py-1 bg-slate-800 rounded text-slate-400 hover:text-red-400"
          @click="deleteAsset('hero')"
        >
          Remove
        </button>
      </div>
      <input
        v-model="heroAlt"
        type="text"
        maxlength="200"
        class="w-full bg-[#0B0E13] border border-slate-800 rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-slate-600"
        placeholder="Alt text for hero image"
      />
    </div>

    <p v-if="uploadError" class="text-xs text-red-400">{{ uploadError }}</p>

    <div class="hidden">{{ logoPreviewUrl }} {{ faviconPreviewUrl }} {{ heroPreviewUrl }}</div>
  </div>
</template>
