import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'

import {
  applyTheme,
  getThemePreference,
  resolveTheme,
  setThemePreference,
  systemPrefersDark,
  type ResolvedTheme,
  type ThemePreference,
} from '@/lib/theme-pref'

type ThemeContextValue = {
  /** What the user picked: light, dark, or follow the OS. */
  preference: ThemePreference
  /** What is actually painted right now. */
  resolved: ResolvedTheme
  setPreference: (pref: ThemePreference) => void
}

const ThemeContext = createContext<ThemeContextValue | null>(null)

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [preference, setPref] = useState<ThemePreference>(getThemePreference)
  const [osDark, setOsDark] = useState<boolean>(systemPrefersDark)

  // Track the OS setting whatever the preference is, so flipping back to
  // "System" paints the right theme immediately instead of one tick late.
  useEffect(() => {
    if (typeof window === 'undefined' || !window.matchMedia) return
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = (e: MediaQueryListEvent) => setOsDark(e.matches)
    setOsDark(mq.matches)
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [])

  const resolved = resolveTheme(preference, osDark)

  useEffect(() => {
    applyTheme(resolved)
  }, [resolved])

  const setPreference = useCallback((next: ThemePreference) => {
    setPref(next)
    try {
      setThemePreference(next)
    } catch {
      // A rejected write (private mode) must not stop the theme switching.
    }
  }, [])

  const value = useMemo(
    () => ({ preference, resolved, setPreference }),
    [preference, resolved, setPreference],
  )

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext)
  if (!ctx) throw new Error('useTheme must be used inside <ThemeProvider>')
  return ctx
}
