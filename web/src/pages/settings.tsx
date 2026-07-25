import { useCallback, useEffect, useState, type FormEvent } from 'react'

import { ThemeToggle } from '@/components/theme-toggle'
import { Button } from '@/components/ui/button'
import {
  changeOwnPassword,
  getSettings,
  updateSettings,
  type AccessSettings,
} from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { cn } from '@/lib/utils'

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
  const [lockBusy, setLockBusy] = useState(false)
  const [lockError, setLockError] = useState<string | null>(null)

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

  async function onToggleTailnetOnly(next: boolean) {
    if (!token || !isAdmin) return
    setLockBusy(true)
    setLockError(null)
    try {
      await updateSettings(token, { tailnet_only: next })
      // Re-read rather than trust the PATCH echo: the diagnostic block only
      // comes back from GET, and it is the part worth looking at.
      await refresh()
    } catch (err) {
      setLockError(err instanceof Error ? err.message : String(err))
    } finally {
      setLockBusy(false)
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

  const access = settings?.access
  // "Real" means the lock would actually refuse someone: enforcement is live
  // and the server can identify callers. Anything else gets a warning, not a tick.
  const lockIsReal =
    Boolean(access?.client_ip_known) && !access?.enforcement_disabled

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

      <section className="flex flex-col gap-3 rounded-lg border border-border bg-card p-4">
        <div>
          <h2 className="text-sm font-semibold">Appearance</h2>
          <p className="mt-1 text-xs text-muted-foreground">
            Applies to this browser only — it is a display preference, not an
            account setting. “System” follows your OS and switches with it.
          </p>
        </div>
        <ThemeToggle showLabels className="w-full max-w-sm" />
      </section>

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

      <section className="flex flex-col gap-3 rounded-lg border border-border bg-card p-4">
        <div>
          <h2 className="text-sm font-semibold">Tailnet-only access</h2>
          <p className="mt-1 text-xs text-muted-foreground">
            When on, this server refuses every request that did not come from
            its Tailscale network — phones included, as long as the Tailscale
            app is connected. Off by default.
          </p>
        </div>

        <label className="flex items-start gap-3 rounded-md border border-border p-3 text-sm">
          <input
            type="checkbox"
            className="mt-0.5 size-4 accent-primary"
            checked={Boolean(settings?.tailnet_only)}
            disabled={!isAdmin || lockBusy}
            onChange={(e) => void onToggleTailnetOnly(e.target.checked)}
          />
          <span>
            <span className="font-medium">
              Only accept requests from the tailnet
            </span>
            <span className="mt-0.5 block text-xs text-muted-foreground">
              You cannot switch this on from an address that would be blocked —
              the server refuses rather than lock you out.
            </span>
          </span>
        </label>

        {access ? (
          <div
            className={cn(
              'rounded-md border p-3 text-xs',
              lockIsReal
                ? 'border-emerald-500/40 bg-emerald-500/10 text-foreground'
                : 'border-amber-500/50 bg-amber-500/10 text-foreground',
            )}
            role="status"
          >
            {access.enforcement_disabled ? (
              <p>
                <strong>Not enforced.</strong> This server runs with{' '}
                <code>ACCESS_ENFORCEMENT=off</code>, so the setting is stored
                but ignored. Remove that variable to make it take effect.
              </p>
            ) : !access.client_ip_known ? (
              <p>
                <strong>This setting cannot protect you yet.</strong> The server
                cannot tell which address requests come from — a reverse proxy
                in front of it is not forwarding the client. Set{' '}
                <code>TRUSTED_PROXIES</code> to those proxies, then reload this
                page.
              </p>
            ) : (
              <p>
                The server sees you at{' '}
                <code className="font-mono">{access.client_ip}</code> —{' '}
                {access.client_on_tailnet
                  ? 'inside the tailnet.'
                  : access.client_allowed
                    ? 'on this machine, which stays allowed either way.'
                    : 'not a Tailscale address, so turning this on would lock you out from here.'}
              </p>
            )}
          </div>
        ) : null}

        {lockError ? (
          <p className="text-sm text-destructive" role="alert">
            {lockError}
          </p>
        ) : null}

        <p className="text-xs text-muted-foreground">
          This is a second lock. The one that actually holds is not publishing
          the port at all — serve the app on the tailnet and there is nothing
          for the internet to reach. See <code>docs/ops.md</code> §5d.
        </p>

        {!isAdmin ? (
          <p className="text-xs text-muted-foreground">
            Only admins can change this.
          </p>
        ) : null}
      </section>

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
