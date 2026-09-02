// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { listCertificates, type CertMonitor } from '@/services/certificateApi'
import { sseBus } from '@/services/sseBus'
import { useResourcesStore } from '@/stores/resources'

export const useCertificatesStore = defineStore('certificates', () => {
  const certificates = ref<CertMonitor[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  const sseConnected = sseBus.connected

  const statusCounts = computed(() => {
    const counts = { valid: 0, expiring: 0, expired: 0, error: 0, unknown: 0 }
    for (const cert of certificates.value) {
      if (cert.status in counts) {
        counts[cert.status as keyof typeof counts]++
      }
    }
    return counts
  })

  async function fetchCertificates() {
    loading.value = true
    error.value = null
    try {
      const res = await listCertificates({ agent_id: useResourcesStore().entityQuery })
      certificates.value = res.certificates || []
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to fetch certificates'
    } finally {
      loading.value = false
    }
  }

  function onCreated() {
    fetchCertificates()
  }

  function onCheckCompleted(e: MessageEvent) {
    let data
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    const idx = certificates.value.findIndex((c) => c.id === data.monitor_id)
    if (idx >= 0) {
      certificates.value[idx] = {
        ...certificates.value[idx]!,
        status: data.status,
        last_check_at: data.checked_at,
      }
    } else {
      fetchCertificates()
    }
  }

  function onStatusChanged(e: MessageEvent) {
    let data
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    const idx = certificates.value.findIndex((c) => c.id === data.monitor_id)
    if (idx >= 0) {
      certificates.value[idx] = {
        ...certificates.value[idx]!,
        status: data.new_status,
      }
    } else {
      fetchCertificates()
    }
  }

  function onAlert() {
    fetchCertificates()
  }

  function onRecovery() {
    fetchCertificates()
  }

  function onDeleted(e: MessageEvent) {
    let data
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    certificates.value = certificates.value.filter((c) => c.id !== data.monitor_id)
  }

  function onReconnected() {
    fetchCertificates()
  }

  function connectSSE() {
    sseBus.on('certificate.created', onCreated)
    sseBus.on('certificate.check_completed', onCheckCompleted)
    sseBus.on('certificate.status_changed', onStatusChanged)
    sseBus.on('certificate.alert', onAlert)
    sseBus.on('certificate.recovery', onRecovery)
    sseBus.on('certificate.deleted', onDeleted)
    sseBus.on('sse.reconnected', onReconnected)
    sseBus.connect()
  }

  function disconnectSSE() {
    sseBus.off('certificate.created', onCreated)
    sseBus.off('certificate.check_completed', onCheckCompleted)
    sseBus.off('certificate.status_changed', onStatusChanged)
    sseBus.off('certificate.alert', onAlert)
    sseBus.off('certificate.recovery', onRecovery)
    sseBus.off('certificate.deleted', onDeleted)
    sseBus.off('sse.reconnected', onReconnected)
    sseBus.disconnect()
  }

  return {
    certificates,
    loading,
    error,
    sseConnected,
    statusCounts,
    fetchCertificates,
    connectSSE,
    disconnectSSE,
  }
})
