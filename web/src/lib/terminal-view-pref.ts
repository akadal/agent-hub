/**
 * Global terminal display mode preference.
 * Shared by all sessions — not per-session / per-machine.
 */

export type TerminalViewMode = 'classic' | 'chat'

export const TERMINAL_VIEW_PREF_KEY = 'agent-hub.terminalViewMode'

export type StorageLike = Pick<Storage, 'getItem' | 'setItem'>

/** In-memory fallback when localStorage is unavailable (SSR / restricted). */
const memoryStore = new Map<string, string>()

function defaultStorage(): StorageLike {
  try {
    if (typeof globalThis !== 'undefined' && 'localStorage' in globalThis) {
      const ls = (globalThis as { localStorage?: Storage }).localStorage
      if (ls) {
        // Probe access (private mode can throw).
        const probe = '__ah_probe__'
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

export function isTerminalViewMode(value: unknown): value is TerminalViewMode {
  return value === 'classic' || value === 'chat'
}

/**
 * Read the global terminal view mode.
 * Defaults to `classic` when unset or invalid.
 */
export function getTerminalViewMode(
  storage: StorageLike = defaultStorage(),
): TerminalViewMode {
  try {
    const raw = storage.getItem(TERMINAL_VIEW_PREF_KEY)
    if (isTerminalViewMode(raw)) return raw
  } catch {
    /* ignore */
  }
  return 'classic'
}

/**
 * Persist the global terminal view mode (all sessions share this).
 */
export function setTerminalViewMode(
  mode: TerminalViewMode,
  storage: StorageLike = defaultStorage(),
): void {
  if (!isTerminalViewMode(mode)) {
    throw new TypeError(`Invalid terminal view mode: ${String(mode)}`)
  }
  storage.setItem(TERMINAL_VIEW_PREF_KEY, mode)
}

/** Toggle classic ↔ chat and return the new mode. */
export function toggleTerminalViewMode(
  storage: StorageLike = defaultStorage(),
): TerminalViewMode {
  const next: TerminalViewMode =
    getTerminalViewMode(storage) === 'chat' ? 'classic' : 'chat'
  setTerminalViewMode(next, storage)
  return next
}
