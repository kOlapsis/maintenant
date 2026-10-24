<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
-->

<script setup lang="ts">
import type { CreateHeartbeatInput } from '@/services/heartbeatApi'

defineProps<{
  initial?: CreateHeartbeatInput
  submitLabel: string
  busy?: boolean
  error?: string | null
}>()

defineEmits<{
  submit: [data: CreateHeartbeatInput]
  cancel: []
}>()

throw new Error('not implemented')
</script>

<template>
  <form />
</template>
