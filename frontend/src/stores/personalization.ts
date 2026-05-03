import { defineStore } from 'pinia'
import { ref } from 'vue'
import {
  personalizationApi,
  type SettingsResponse,
  type FooterLink,
  type FAQItem,
  type ContrastWarning,
} from '@/services/personalizationApi'

export const usePersonalizationStore = defineStore('personalization', () => {
  const settings = ref<SettingsResponse | null>(null)
  const footerLinks = ref<FooterLink[]>([])
  const faqItems = ref<FAQItem[]>([])
  const contrastWarnings = ref<ContrastWarning[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function fetchSettings() {
    loading.value = true
    error.value = null
    try {
      settings.value = await personalizationApi.getSettings()
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to load settings'
    } finally {
      loading.value = false
    }
  }

  async function saveSettings(payload: Parameters<typeof personalizationApi.putSettings>[0]) {
    loading.value = true
    error.value = null
    try {
      const res = await personalizationApi.putSettings(payload)
      settings.value = res
      contrastWarnings.value = res.warnings?.contrast ?? []
      return res
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to save settings'
      throw e
    } finally {
      loading.value = false
    }
  }

  async function fetchFooterLinks() {
    const res = await personalizationApi.listFooterLinks()
    footerLinks.value = res.items ?? []
  }

  async function createFooterLink(label: string, url: string) {
    const link = await personalizationApi.createFooterLink(label, url)
    footerLinks.value.push(link)
    return link
  }

  async function updateFooterLink(id: number, label: string, url: string) {
    const link = await personalizationApi.updateFooterLink(id, label, url)
    const idx = footerLinks.value.findIndex((l) => l.id === id)
    if (idx !== -1) footerLinks.value[idx] = link
    return link
  }

  async function deleteFooterLink(id: number) {
    await personalizationApi.deleteFooterLink(id)
    footerLinks.value = footerLinks.value.filter((l) => l.id !== id)
  }

  async function reorderFooterLinks(ids: number[]) {
    const res = await personalizationApi.reorderFooterLinks(ids)
    footerLinks.value = res.items ?? []
  }

  async function fetchFAQ() {
    const res = await personalizationApi.listFAQ()
    faqItems.value = res.items ?? []
  }

  async function createFAQItem(question: string, answerMD: string) {
    const item = await personalizationApi.createFAQItem(question, answerMD)
    faqItems.value.push(item)
    return item
  }

  async function updateFAQItem(id: number, question: string, answerMD: string) {
    const item = await personalizationApi.updateFAQItem(id, question, answerMD)
    const idx = faqItems.value.findIndex((f) => f.id === id)
    if (idx !== -1) faqItems.value[idx] = item
    return item
  }

  async function deleteFAQItem(id: number) {
    await personalizationApi.deleteFAQItem(id)
    faqItems.value = faqItems.value.filter((f) => f.id !== id)
  }

  async function reorderFAQ(ids: number[]) {
    const res = await personalizationApi.reorderFAQ(ids)
    faqItems.value = res.items ?? []
  }

  async function uploadAsset(role: string, file: File, altText?: string) {
    return personalizationApi.putAsset(role, file, altText)
  }

  async function deleteAsset(role: string) {
    return personalizationApi.deleteAsset(role)
  }

  return {
    settings,
    footerLinks,
    faqItems,
    contrastWarnings,
    loading,
    error,
    fetchSettings,
    saveSettings,
    fetchFooterLinks,
    createFooterLink,
    updateFooterLink,
    deleteFooterLink,
    reorderFooterLinks,
    fetchFAQ,
    createFAQItem,
    updateFAQItem,
    deleteFAQItem,
    reorderFAQ,
    uploadAsset,
    deleteAsset,
  }
})
