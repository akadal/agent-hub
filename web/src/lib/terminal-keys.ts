/**
 * Key translation for the mobile soft-key bar.
 *
 * A phone keyboard has no Ctrl. The bar arms a sticky Ctrl instead, and the
 * next character the OS keyboard produces is folded into its control code
 * here before it reaches the PTY.
 */

/**
 * Ctrl+<key> as the byte a terminal expects, or null when the key has no
 * control form (digits, punctuation outside the classic range) — in which
 * case the caller sends the key unchanged rather than swallowing it.
 */
export function controlSequence(input: string): string | null {
  if (input.length !== 1) return null
  const code = input.charCodeAt(0)
  // @ A-Z [ \ ] ^ _  → 0x00–0x1f, the classic control range.
  if (code >= 0x40 && code <= 0x5f) return String.fromCharCode(code - 0x40)
  // Lowercase maps to the same control code as its uppercase (Ctrl+c = Ctrl+C).
  if (code >= 0x61 && code <= 0x7a) return String.fromCharCode(code - 0x60)
  if (input === ' ') return '\x00'
  // Ctrl+? is DEL, the one control character above the range.
  if (input === '?') return '\x7f'
  return null
}
