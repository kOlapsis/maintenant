// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

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
