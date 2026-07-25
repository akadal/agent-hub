import { useCallback, useEffect, useState, type FormEvent } from 'react'

import { Button } from '@/components/ui/button'
import {
  changeOwnPassword,
  getSettings,
  updateSettings,
  type AccessSettings,
} from '@/lib/api'
import { useAuth } from '@/lib/auth'

export function SettingsPage() {
  const { token, user } = useAuth()
  const isAdmin = user?.role === 'admin'
  const [settings, setSettings] = useState<AccessSettings | null>(null)
  const [mode, setMode] = useState('private_mesh')
  const [error, setError] = useState<string | null>(null)
  const [saved, setSaved] = useState(false)
  const [busy, setBusy] = useState(false)
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [pwError, setPwError] = useState<string | null>(null)
  const [pwDone, setPwDone] = useState(false)
  const [pwBusy, setPwBusy] = useState(false)

  const refresh = useCallback(async () => {
    if (!token) return
    setError(null)
    try {
      const s = await getSettings(token)
      setSettings(s)
      setMode(s.network_mode || 'private_mesh')
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }, [token])

  useEffect(() => {
    void refresh()
  }, [refresh])

  async function onSave(e: FormEvent) {
    e.preventDefault()
    if (!token || !isAdmin) return
    setBusy(true)
    setError(null)
    setSaved(false)
    try {
      const s = await updateSettings(token, { network_mode: mode })
      setSettings(s)
      setMode(s.network_mode)
      setSaved(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  async function onChangePassword(e: FormEvent) {
    e.preventDefault()
    if (!token) return
    setPwError(null)
    setPwDone(false)
    if (newPassword !== confirmPassword) {
      setPwError('The two new password fields do not match.')
      return
    }
    setPwBusy(true)
    try {
      await changeOwnPassword(token, {
        current_password: currentPassword,
        new_password: newPassword,
      })
      setCurrentPassword('')
      setNewPassword('')
      setConfirmPassword('')
      setPwDone(true)
    } catch (err) {
      setPwError(err instanceof Error ? err.message : String(err))
    } finally {
      setPwBusy(false)
    }
  }

  return (
    <div className="mx-auto flex max-w-2xl flex-col gap-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Settings</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Your account, and the access policy intent for this instance. Network
          edge enforcement is still done by your reverse proxy / Tailscale /
          firewall — the app records the chosen default for operators and audit.
        </p>
      </div>

      <form
        onSubmit={(e) => void onChangePassword(e)}
        className="flex flex-col gap-4 rounded-lg border border-border bg-card p-4"
      >
        <div>
          <h2 className="text-sm font-semibold">
            Change password
            {user?.username ? (
              <span className="font-normal text-muted-foreground">
                {' '}
                — {user.username}
              </span>
            ) : null}
          </h2>
          <p className="mt-1 text-xs text-muted-foreground">
            Changing your own password requires the current one. Admins can also
            reset any account from the Users page.
          </p>
        </div>

        <label className="flex flex-col gap-1 text-sm">
          <span className="text-muted-foreground">Current password</span>
          <input
            type="password"
            autoComplete="current-password"
            required
            className="h-9 rounded-md border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring"
            value={currentPassword}
            onChange={(e) => {
              setCurrentPassword(e.target.value)
              setPwDone(false)
            }}
          />
        </label>
        <label className="flex flex-col gap-1 text-sm">
          <span className="text-muted-foreground">New password</span>
          <input
            type="password"
            autoComplete="new-password"
            required
            className="h-9 rounded-md border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring"
            value={newPassword}
            onChange={(e) => {
              setNewPassword(e.target.value)
              setPwDone(false)
            }}
          />
        </label>
        <label className="flex flex-col gap-1 text-sm">
          <span className="text-muted-foreground">Confirm new password</span>
          <input
            type="password"
            autoComplete="new-password"
            required
            className="h-9 rounded-md border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring"
            value={confirmPassword}
            onChange={(e) => {
              setConfirmPassword(e.target.value)
              setPwDone(false)
            }}
          />
        </label>

        {pwError ? (
          <p className="text-sm text-destructive" role="alert">
            {pwError}
          </p>
        ) : null}

        <div className="flex items-center gap-3">
          <Button type="submit" disabled={pwBusy}>
            {pwBusy ? 'Changing…' : 'Change password'}
          </Button>
          {pwDone ? (
            <span className="text-xs text-muted-foreground">
              Password changed. Your current session stays signed in.
            </span>
          ) : null}
        </div>
      </form>

      {error ? (
        <p className="text-sm text-destructive" role="alert">
          {error}
        </p>
      ) : null}

      <form
        onSubmit={(e) => void onSave(e)}
        className="flex flex-col gap-4 rounded-lg border border-border bg-card p-4"
      >
        <h2 className="text-sm font-semibold">Network access policy</h2>
        <label className="flex flex-col gap-1 text-sm">
          <span className="text-muted-foreground">Mode</span>
          <select
            className="h-9 rounded-md border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring disabled:opacity-60"
            value={mode}
            onChange={(e) => {
              setMode(e.target.value)
              setSaved(false)
            }}
            disabled={!isAdmin}
          >
            <option value="private_mesh">
              private_mesh — Tailscale / private network only (default)
            </option>
            <option value="open">
              open — intentionally wider access (operator secures the edge)
            </option>
          </select>
        </label>

        <div className="rounded-md border border-dashed border-border bg-muted/30 p-3 text-xs text-muted-foreground">
          <p className="font-medium text-foreground">Operator checklist</p>
          <ul className="mt-2 list-disc space-y-1 pl-4">
            <li>
              Prefer putting the stack on a private mesh (e.g. Tailscale) and not
              exposing ports to the public internet.
            </li>
            <li>
              If you publish HTTPS, use TLS at the reverse proxy and keep SSH
              targets on the mesh only.
            </li>
            <li>
              Current stored mode:{' '}
              <code className="text-foreground">
                {settings?.network_mode ?? '…'}
              </code>
            </li>
          </ul>
        </div>

        {isAdmin ? (
          <div className="flex items-center gap-3">
            <Button type="submit" disabled={busy}>
              {busy ? 'Saving…' : 'Save'}
            </Button>
            {saved ? (
              <span className="text-xs text-muted-foreground">Saved.</span>
            ) : null}
          </div>
        ) : (
          <p className="text-xs text-muted-foreground">
            Only admins can change policy. You can view the current mode above.
          </p>
        )}
      </form>
    </div>
  )
}
