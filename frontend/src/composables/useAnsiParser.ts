/*
  Copyright 2026 Benjamin Touchard (kOlapsis)

  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. You may not use this file except in compliance
  with one of these licenses.

  AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
  Commercial: See COMMERCIAL-LICENSE.md

  Source: https://github.com/kolapsis/maintenant
*/

export interface AnsiStyle {
  fg: string | null
  bg: string | null
  bold: boolean
  dim: boolean
  italic: boolean
  underline: boolean
  strikethrough: boolean
}

export interface AnsiToken {
  text: string
  style: AnsiStyle
}

const STANDARD_COLORS: Record<number, string> = {
  0: '#000000', 1: '#aa0000', 2: '#00aa00', 3: '#aa5500',
  4: '#0000aa', 5: '#aa00aa', 6: '#00aaaa', 7: '#aaaaaa',
}

const BRIGHT_COLORS: Record<number, string> = {
  0: '#555555', 1: '#ff5555', 2: '#55ff55', 3: '#ffff55',
  4: '#5555ff', 5: '#ff55ff', 6: '#55ffff', 7: '#ffffff',
}

function color256(n: number): string | null {
  if (n < 0 || n > 255) return null
  if (n < 8) return STANDARD_COLORS[n] ?? null
  if (n < 16) return BRIGHT_COLORS[n - 8] ?? null
  if (n < 232) {
    const idx = n - 16
    const r = Math.floor(idx / 36)
    const g = Math.floor((idx % 36) / 6)
    const b = idx % 6
    const toHex = (v: number) => (v === 0 ? 0 : 55 + v * 40)
    return `#${toHex(r).toString(16).padStart(2, '0')}${toHex(g).toString(16).padStart(2, '0')}${toHex(b).toString(16).padStart(2, '0')}`
  }
  const gray = 8 + (n - 232) * 10
  return `#${gray.toString(16).padStart(2, '0')}${gray.toString(16).padStart(2, '0')}${gray.toString(16).padStart(2, '0')}`
}

function defaultStyle(): AnsiStyle {
  return { fg: null, bg: null, bold: false, dim: false, italic: false, underline: false, strikethrough: false }
}

function cloneStyle(s: AnsiStyle): AnsiStyle {
  return { fg: s.fg, bg: s.bg, bold: s.bold, dim: s.dim, italic: s.italic, underline: s.underline, strikethrough: s.strikethrough }
}

const SGR_RE = /\x1b\[([\d;]*)m/g

function applySgrCodes(codes: number[], style: AnsiStyle): void {
  let i = 0
  while (i < codes.length) {
    const c = codes[i]!
    if (c === 0) {
      Object.assign(style, defaultStyle())
    } else if (c === 1) {
      style.bold = true
    } else if (c === 2) {
      style.dim = true
    } else if (c === 3) {
      style.italic = true
    } else if (c === 4) {
      style.underline = true
    } else if (c === 9) {
      style.strikethrough = true
    } else if (c === 22) {
      style.bold = false
      style.dim = false
    } else if (c === 23) {
      style.italic = false
    } else if (c === 24) {
      style.underline = false
    } else if (c === 29) {
      style.strikethrough = false
    } else if (c >= 30 && c <= 37) {
      style.fg = STANDARD_COLORS[c - 30] ?? null
    } else if (c === 38) {
      if (i + 1 < codes.length && codes[i + 1] === 5 && i + 2 < codes.length) {
        style.fg = color256(codes[i + 2]!)
        i += 2
      } else if (i + 1 < codes.length && codes[i + 1] === 2 && i + 4 < codes.length) {
        const r = Math.min(255, Math.max(0, codes[i + 2]!))
        const g = Math.min(255, Math.max(0, codes[i + 3]!))
        const b = Math.min(255, Math.max(0, codes[i + 4]!))
        style.fg = `#${r.toString(16).padStart(2, '0')}${g.toString(16).padStart(2, '0')}${b.toString(16).padStart(2, '0')}`
        i += 4
      }
    } else if (c === 39) {
      style.fg = null
    } else if (c >= 40 && c <= 47) {
      style.bg = STANDARD_COLORS[c - 40] ?? null
    } else if (c === 48) {
      if (i + 1 < codes.length && codes[i + 1] === 5 && i + 2 < codes.length) {
        style.bg = color256(codes[i + 2]!)
        i += 2
      } else if (i + 1 < codes.length && codes[i + 1] === 2 && i + 4 < codes.length) {
        const r = Math.min(255, Math.max(0, codes[i + 2]!))
        const g = Math.min(255, Math.max(0, codes[i + 3]!))
        const b = Math.min(255, Math.max(0, codes[i + 4]!))
        style.bg = `#${r.toString(16).padStart(2, '0')}${g.toString(16).padStart(2, '0')}${b.toString(16).padStart(2, '0')}`
        i += 4
      }
    } else if (c === 49) {
      style.bg = null
    } else if (c >= 90 && c <= 97) {
      style.fg = BRIGHT_COLORS[c - 90] ?? null
    } else if (c >= 100 && c <= 107) {
      style.bg = BRIGHT_COLORS[c - 100] ?? null
    }
    i++
  }
}

export function parseAnsi(input: string): AnsiToken[] {
  if (!input) return []

  const tokens: AnsiToken[] = []
  const style = defaultStyle()
  let lastIndex = 0

  SGR_RE.lastIndex = 0
  let match: RegExpExecArray | null

  while ((match = SGR_RE.exec(input)) !== null) {
    if (match.index > lastIndex) {
      tokens.push({ text: input.slice(lastIndex, match.index), style: cloneStyle(style) })
    }

    const raw = match[1] ?? ''
    const codes = raw === '' ? [0] : raw.split(';').map(Number)
    applySgrCodes(codes, style)

    lastIndex = SGR_RE.lastIndex
  }

  if (lastIndex < input.length) {
    tokens.push({ text: input.slice(lastIndex), style: cloneStyle(style) })
  }

  // Strip any remaining non-SGR escape sequences from token text
  for (const token of tokens) {
    token.text = token.text.replace(/\x1b\[[^a-zA-Z]*[a-zA-Z]/g, '')
  }

  return tokens.filter(t => t.text.length > 0)
}
