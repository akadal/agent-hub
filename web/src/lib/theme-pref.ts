/**
 * Appearance preference: light, dark, or follow the OS.
 *
 * Storage holds the *preference* ("system"), never the resolved value —
 * otherwise a laptop that switches to dark at sunset would stay light
 * because we had frozen the answer at first visit.
 */

export type ThemePreference = 'light' | 'dark' | 'system'
/** What actually gets painted — `system` is resolved away before this. */
export type ResolvedTheme = 'light' | 'dark'

export const THEME_PREF_KEY = 'agent-hub.theme'

export type StorageLike = Pick<Storage, 'getItem' | 'setItem'>

/** In-memory fallback when localStorage is unavailable (SSR / private mode). */
const memoryStore = new Map<string, string>()

function defaultStorage(): StorageLike {
  try {
    if (typeof globalThis !== 'undefined' && 'localStorage' in globalThis) {
      const ls = (globalThis as { localStorage?: Storage }).localStorage
      if (ls) {
        const probe = '__ah_theme_probe__'
        ls.setItem(probe, '1')
        ls.removeItem(probe)
        return ls
      }
    }
  } catch {
    /* fall through */
  }
  return {
    getItem: (k) => memoryStore.get(k) ?? null,
    setItem: (k, v) => {
      memoryStore.set(k, v)
    },
  }
}

export function isThemePreference(value: unknown): value is ThemePreference {
  return value === 'light' || value === 'dark' || value === 'system'
}

/** Read the stored preference. Unset or corrupt storage means `system`. */
export function getThemePreference(
  storage: StorageLike = defaultStorage(),
): ThemePreference {
  try {
    const raw = storage.getItem(THEME_PREF_KEY)
    if (isThemePreference(raw)) return raw
  } catch {
    /* ignore */
  }
  return 'system'
}

export function setThemePreference(
  pref: ThemePreference,
  storage: StorageLike = defaultStorage(),
): void {
  if (!isThemePreference(pref)) {
    throw new TypeError(`Invalid theme preference: ${String(pref)}`)
  }
  storage.setItem(THEME_PREF_KEY, pref)
}

/** Collapse the preference against the OS setting into a paintable theme. */
export function resolveTheme(
  pref: ThemePreference,
  systemPrefersDark: boolean,
): ResolvedTheme {
  if (pref === 'system') return systemPrefersDark ? 'dark' : 'light'
  return pref
}

/** True when the OS asks for dark. False anywhere `matchMedia` is missing. */
export function systemPrefersDark(): boolean {
  if (typeof window === 'undefined' || !window.matchMedia) return false
  return window.matchMedia('(prefers-color-scheme: dark)').matches
}

/**
 * Paint the theme onto the document.
 *
 * `color-scheme` is not cosmetic: without it the browser keeps painting form
 * controls, scrollbars and the rubber-band overscroll area light, which on a
 * phone shows up as white flashes around a dark terminal.
 */
export function applyTheme(
  theme: ResolvedTheme,
  root: { classList: DOMTokenList; style: CSSStyleDeclaration } | null = typeof document !==
  'undefined'
    ? document.documentElement
    : null,
): void {
  if (!root) return
  root.classList.toggle('dark', theme === 'dark')
  root.style.colorScheme = theme
  // Matches the surface behind the app so the phone's status bar and the
  // area revealed by overscroll do not stay the other theme's colour.
  if (typeof document !== 'undefined') {
    const meta = document.querySelector('meta[name="theme-color"]')
    if (meta) {
      meta.setAttribute('content', theme === 'dark' ? '#0a0a0a' : '#ffffff')
    }
  }
}
