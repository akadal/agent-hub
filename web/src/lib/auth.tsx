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
  fetchMe,
  isAuthRejection,
  login as apiLogin,
  type User,
} from '@/lib/api'

const TOKEN_KEY = 'agent-hub-token'

/**
 * Session restore retries a transient API failure instead of logging the user
 * out. Only a 401/403 means the token is actually dead; a redeploy, a proxy
 * blip or a phone waking from sleep must not destroy a working session.
 */
const RESTORE_RETRY_DELAYS_MS = [1000, 2000, 4000, 8000]

const sleep = (ms: number) =>
  new Promise<void>((resolve) => setTimeout(resolve, ms))

type AuthState = {
  token: string | null
  user: User | null
  loading: boolean
  login: (username: string, password: string) => Promise<void>
  logout: () => void
}

const AuthContext = createContext<AuthState | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [token, setToken] = useState<string | null>(() =>
    localStorage.getItem(TOKEN_KEY),
  )
  const [user, setUser] = useState<User | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let cancelled = false
    async function load() {
      if (!token) {
        setUser(null)
        setLoading(false)
        return
      }
      for (let attempt = 0; ; attempt++) {
        try {
          const me = await fetchMe(token)
          if (!cancelled) {
            setUser(me)
            setLoading(false)
          }
          return
        } catch (err) {
          if (cancelled) return
          if (isAuthRejection(err)) {
            // The token really is dead — drop it and send the user to login.
            localStorage.removeItem(TOKEN_KEY)
            setToken(null)
            setUser(null)
            setLoading(false)
            return
          }
          if (attempt >= RESTORE_RETRY_DELAYS_MS.length) {
            // Still failing, but the token was never rejected. Keep it in
            // storage so a reload recovers the session once the API is back.
            setUser(null)
            setLoading(false)
            return
          }
          await sleep(RESTORE_RETRY_DELAYS_MS[attempt])
        }
      }
    }
    void load()
    return () => {
      cancelled = true
    }
  }, [token])

  const login = useCallback(async (username: string, password: string) => {
    const res = await apiLogin(username, password)
    localStorage.setItem(TOKEN_KEY, res.token)
    setToken(res.token)
    setUser(res.user)
  }, [])

  const logout = useCallback(() => {
    localStorage.removeItem(TOKEN_KEY)
    setToken(null)
    setUser(null)
  }, [])

  const value = useMemo(
    () => ({ token, user, loading, login, logout }),
    [token, user, loading, login, logout],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within AuthProvider')
  return ctx
}
