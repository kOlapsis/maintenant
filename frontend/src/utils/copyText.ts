// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

import { showToast } from '@/composables/useToast'

function copyWithSelection(text: string): boolean {
  const area = document.createElement('textarea')
  area.value = text
  area.style.position = 'fixed'
  area.style.opacity = '0'
  document.body.appendChild(area)
  area.select()
  try {
    return document.execCommand('copy')
  } catch {
    return false
  } finally {
    area.remove()
  }
}

/** Copies text to the clipboard, tells the user when it could not, and returns whether it did. */
export async function copyText(text: string): Promise<boolean> {
  try {
    // navigator.clipboard is undefined on a page served over plain HTTP.
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    const copied = copyWithSelection(text)
    if (!copied) showToast('Could not copy. Select the text and copy it by hand.', 'warning')
    return copied
  }
}
