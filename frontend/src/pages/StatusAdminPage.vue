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
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useStatusAdminStore } from '@/stores/statusAdmin'
import { usePersonalizationStore } from '@/commercial/stores/personalization'
import { useEdition } from '@/composables/useEdition'
import StatusComponentManager from '@/components/StatusComponentManager.vue'
import StatusIncidentManager from '@/commercial/components/status/StatusIncidentManager.vue'
import StatusSmtpConfig from '@/commercial/components/status/StatusSmtpConfig.vue'
import StatusMaintenanceManager from '@/commercial/components/status/StatusMaintenanceManager.vue'
import SubscribersPanel from '@/commercial/components/status/SubscribersPanel.vue'
import FeatureGate from '@/components/FeatureGate.vue'
import SmtpNotConfigured from '@/components/SmtpNotConfigured.vue'
import BrandingSection from '@/commercial/components/personalization/BrandingSection.vue'
import ColorPaletteSection from '@/commercial/components/personalization/ColorPaletteSection.vue'
import HeaderAnnouncementSection from '@/commercial/components/personalization/HeaderAnnouncementSection.vue'
import FooterSection from '@/commercial/components/personalization/FooterSection.vue'
import FaqSection from '@/commercial/components/personalization/FaqSection.vue'
import LocalizationSection from '@/commercial/components/personalization/LocalizationSection.vue'
import type { PalettePayload } from '@/commercial/services/personalizationApi'
import TabNav, { type TabNavItem } from '@/components/ui/TabNav.vue'
import UiButton from '@/components/ui/UiButton.vue'

const { hasFeature, editionPermits, statusURL } = useEdition()

const store = useStatusAdminStore()
const persoStore = usePersonalizationStore()

const brandingRef = ref<InstanceType<typeof BrandingSection> | null>(null)

type Tab = 'components' | 'incidents' | 'maintenance' | 'subscribers' | 'smtp' | 'personalization'
const activeTab = ref<Tab>('components')

// --- Personalization form state ---
const persoSaving = ref(false)
const persoSaveError = ref('')
const persoSaveSuccess = ref(false)
const persoLoaded = ref(false)

const title = ref('System Status')
const subtitle = ref('')
const palette = ref<PalettePayload>({
  bg: '#0B0E13',
  surface: '#12151C',
  border: '#1F2937',
  text: '#FFFFFF',
  accent: '#22C55E',
  status_operational: '#22C55E',
  status_degraded: '#EAB308',
  status_partial: '#F97316',
  status_major: '#EF4444',
})
const announcementEnabled = ref(false)
const announcementMD = ref('')
const announcementURL = ref('')
const footerTextMD = ref('')
const locale = ref('en')
const timezone = ref('')
const dateFormat = ref('relative')

const contrastWarnings = computed(() => persoStore.contrastWarnings)

function syncFromStore() {
  const s = persoStore.settings
  if (!s) return
  title.value = s.title
  subtitle.value = s.subtitle
  palette.value = { ...s.colors }
  announcementEnabled.value = s.announcement.enabled
  announcementMD.value = s.announcement.message_md
  announcementURL.value = s.announcement.url
  footerTextMD.value = s.footer_text_md
  locale.value = s.locale
  timezone.value = s.timezone
  dateFormat.value = s.date_format
}

async function loadPersonalization() {
  if (persoLoaded.value) return
  await persoStore.fetchSettings()
  await persoStore.fetchFooterLinks()
  await persoStore.fetchFAQ()
  syncFromStore()
  persoLoaded.value = true
}

watch(activeTab, (tab) => {
  if (tab === 'personalization') void loadPersonalization()
})

async function savePersonalization() {
  persoSaving.value = true
  persoSaveError.value = ''
  persoSaveSuccess.value = false
  try {
    await persoStore.saveSettings({
      title: title.value,
      subtitle: subtitle.value,
      colors: palette.value,
      announcement: {
        enabled: announcementEnabled.value,
        message_md: announcementMD.value,
        url: announcementURL.value,
      },
      footer_text_md: footerTextMD.value,
      locale: locale.value,
      timezone: timezone.value,
      date_format: dateFormat.value,
    })
    await brandingRef.value?.flushPendingAssets()
    persoSaveSuccess.value = true
    setTimeout(() => (persoSaveSuccess.value = false), 3000)
  } catch (e) {
    persoSaveError.value = e instanceof Error ? e.message : 'Failed to save'
  } finally {
    persoSaving.value = false
  }
}

onMounted(() => {
  store.fetchComponents()
  if (hasFeature('incidents')) store.fetchIncidents()
  if (hasFeature('maintenance_windows')) store.fetchMaintenance()
  if (hasFeature('subscribers')) store.fetchSubscribers()
  store.connectSSE()
})

onUnmounted(() => {
  store.disconnectSSE()
})

const tabItems = computed<TabNavItem<Tab>[]>(() => [
  { value: 'components', label: 'Components', count: store.components?.length || undefined },
  { value: 'incidents', label: 'Incidents', count: store.incidentsTotal || undefined },
  { value: 'maintenance', label: 'Maintenance', count: store.maintenance?.length || undefined },
  {
    value: 'subscribers',
    label: 'Subscribers',
    count: store.subscriberTotal ? `${store.subscriberConfirmed}/${store.subscriberTotal}` : undefined,
  },
  { value: 'smtp', label: 'SMTP' },
  { value: 'personalization', label: 'Personalization' },
])
</script>

<template>
  <div class="overflow-y-auto p-3 sm:p-6">
  <div class="max-w-7xl mx-auto">
    <div class="mb-6">
      <h1 class="text-2xl font-black text-mnt-primary">Status Page</h1>
      <p class="mt-1 text-sm" style="color: var(--mnt-text-muted)">
        Manage the public status page components, incidents, and maintenance windows
      </p>
      <a
        :href="statusURL || '/status'"
        target="_blank"
        class="mt-1 inline-block text-sm transition-colors"
        style="color: var(--mnt-accent)"
        @mouseenter="($event.target as HTMLElement).style.color = 'var(--mnt-accent-hover)'"
        @mouseleave="($event.target as HTMLElement).style.color = 'var(--mnt-accent)'"
      >
        View public status page &rarr;
      </a>
    </div>

    <!-- Tab navigation -->
    <TabNav
      v-model="activeTab"
      class="mb-4"
      :items="tabItems"
      ariaLabel="Status page sections"
    />

    <!-- Tab content -->
    <StatusComponentManager v-if="activeTab === 'components'" />
    <FeatureGate v-else-if="activeTab === 'incidents'" feature="incidents" title="Incident Management" description="Track and communicate outages in real time. Your users see a live timeline of what happened, what's being done, and when it's resolved.">
      <StatusIncidentManager />
    </FeatureGate>
    <FeatureGate v-else-if="activeTab === 'maintenance'" feature="maintenance_windows" title="Maintenance Windows" description="Schedule maintenance ahead of time and notify your users automatically. No more surprise downtime.">
      <StatusMaintenanceManager />
    </FeatureGate>
    <FeatureGate v-else-if="activeTab === 'subscribers'" feature="subscribers" title="Subscriber Notifications" description="Let your users subscribe to status updates by email. They get notified instantly when an incident starts or a maintenance is planned.">
      <SubscribersPanel />
    </FeatureGate>
    <FeatureGate v-else-if="activeTab === 'smtp'" feature="smtp" title="SMTP Configuration" description="Use your own mail server to send notifications. Full control over sender address, branding, and deliverability.">
      <StatusSmtpConfig />
      <template v-if="editionPermits('smtp')" #placeholder>
        <SmtpNotConfigured />
      </template>
    </FeatureGate>

    <!-- Personalization tab -->
    <FeatureGate
      v-else-if="activeTab === 'personalization'"
      feature="personalization"
      title="Status Page Personalization"
      description="Customize your public status page with your brand colors, logo, announcements, FAQ, and localization."
    >
      <div class="space-y-6">
        <!-- Save bar -->
        <div class="flex items-center justify-end gap-3">
          <span v-if="persoSaveSuccess" class="text-xs" style="color: var(--mnt-status-ok)">Saved!</span>
          <span v-if="persoSaveError" class="text-xs" style="color: var(--mnt-status-error)">{{ persoSaveError }}</span>
          <UiButton variant="primary" :loading="persoSaving" @click="savePersonalization">
            {{ persoSaving ? 'Saving…' : 'Save changes' }}
          </UiButton>
        </div>

        <!-- Row 1: Branding (left) + Colors (right) -->
        <div class="grid grid-cols-1 xl:grid-cols-2 gap-6 items-start">
          <div
            class="rounded-xl border p-6"
            style="background: var(--mnt-bg-surface); border-color: var(--mnt-border-default)"
          >
            <BrandingSection ref="brandingRef" v-model:title="title" v-model:subtitle="subtitle" />
          </div>
          <div
            class="rounded-xl border p-6"
            style="background: var(--mnt-bg-surface); border-color: var(--mnt-border-default)"
          >
            <ColorPaletteSection v-model:palette="palette" v-model:warnings="contrastWarnings" />
          </div>
        </div>

        <!-- Row 2: Editorial content (full width) -->
        <div
          class="rounded-xl border p-6 space-y-6"
          style="background: var(--mnt-bg-surface); border-color: var(--mnt-border-default)"
        >
          <HeaderAnnouncementSection
            v-model:enabled="announcementEnabled"
            v-model:message-m-d="announcementMD"
            v-model:url="announcementURL"
          />
          <hr style="border-color: var(--mnt-border-default)" />
          <FooterSection v-model:footer-text-m-d="footerTextMD" />
          <hr style="border-color: var(--mnt-border-default)" />
          <FaqSection />
        </div>

        <!-- Row 3: Localization (full width) -->
        <div
          class="rounded-xl border p-6"
          style="background: var(--mnt-bg-surface); border-color: var(--mnt-border-default)"
        >
          <LocalizationSection
            v-model:locale="locale"
            v-model:timezone="timezone"
            v-model:date-format="dateFormat"
          />
        </div>
      </div>
    </FeatureGate>
  </div>
  </div>
</template>
