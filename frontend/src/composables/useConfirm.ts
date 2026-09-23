// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

import { ref, type InjectionKey, inject, provide } from 'vue'

export interface ConfirmOptions {
  title: string
  message: string
  confirmLabel?: string
  cancelLabel?: string
  destructive?: boolean
}

export interface ConfirmState extends ConfirmOptions {
  resolve: (value: boolean) => void
}

const state = ref<ConfirmState | null>(null)

export const confirmKey: InjectionKey<{
  confirm: (opts: ConfirmOptions) => Promise<boolean>
  state: typeof state
}> = Symbol('confirm')

export function provideConfirm() {
  function confirm(opts: ConfirmOptions): Promise<boolean> {
    return new Promise((resolve) => {
      state.value = {
        ...opts,
        resolve(value: boolean) {
          state.value = null
          resolve(value)
        },
      }
    })
  }

  provide(confirmKey, { confirm, state })

  return { confirm, state }
}

export function useConfirm() {
  const ctx = inject(confirmKey)
  if (!ctx) {
    throw new Error('useConfirm() requires provideConfirm() in a parent component')
  }
  return ctx.confirm
}
