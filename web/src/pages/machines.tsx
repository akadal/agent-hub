import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { LayoutPanelLeft, Plus, Trash2, Network } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  createMachine,
  deleteMachine,
  execOnMachine,
  getTailscaleStatus,
  importFromTailscale,
  listMachines,
  type Machine,
  type TailscaleStatus,
} from '@/lib/api'
import { useAuth } from '@/lib/auth'

export function MachinesPage() {
  const { token } = useAuth()
  const [machines, setMachines] = useState<Machine[]>([])
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [name, setName] = useState('ssh-dummy')
  const [address, setAddress] = useState('ssh-target')
  const [port, setPort] = useState(27343)
  const [sshUser, setSshUser] = useState('root')
  const [sshPassword, setSshPassword] = useState('targetpass')
  const [busy, setBusy] = useState(false)
  const [execOut, setExecOut] = useState<string | null>(null)

  // Tailscale import panel
  const [tsOpen, setTsOpen] = useState(false)
  const [tsStatus, setTsStatus] = useState<TailscaleStatus | null>(null)
  const [tsLoading, setTsLoading] = useState(false)
  const [tsUser, setTsUser] = useState('root')
  const [tsPassword, setTsPassword] = useState('')
  const [tsPort, setTsPort] = useState(22)
  const [tsOnlineOnly, setTsOnlineOnly] = useState(true)
  const [tsBusy, setTsBusy] = useState(false)
  const [tsMsg, setTsMsg] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    if (!token) return
    setLoading(true)
    setError(null)
    try {
      setMachines(await listMachines(token))
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }, [token])

  useEffect(() => {
    void refresh()
  }, [refresh])

  async function openTailscale() {
    if (!token) return
    setTsOpen(true)
    setTsMsg(null)
    setTsLoading(true)
    setError(null)
    try {
      setTsStatus(await getTailscaleStatus(token))
    } catch (e) {
      setTsStatus(null)
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setTsLoading(false)
    }
  }

  async function onImportTailscale() {
    if (!token) return
    setTsBusy(true)
    setTsMsg(null)
    setError(null)
    try {
      const res = await importFromTailscale(token, {
        ssh_user: tsUser,
        ssh_password: tsPassword,
        port: tsPort,
        online_only: tsOnlineOnly,
      })
      setTsMsg(res.message)
      // Refresh device list + machines
      setTsStatus(await getTailscaleStatus(token))
      await refresh()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setTsBusy(false)
    }
  }

  async function onCreate(e: FormEvent) {
    e.preventDefault()
    if (!token) return
    setBusy(true)
    setError(null)
    try {
      await createMachine(token, {
        name,
        address,
        port,
        ssh_user: sshUser,
        ssh_password: sshPassword,
      })
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  async function onDelete(id: string, name: string) {
    if (!token) return
    if (
      !window.confirm(
        `Delete machine “${name}”? Active sessions on this host will be lost and this cannot be undone.`,
      )
    ) {
      return
    }
    await deleteMachine(token, id)
    await refresh()
  }

  async function onProbe(id: string) {
    if (!token) return
    setExecOut(null)
    try {
      const res = await execOnMachine(token, id, 'echo agent-hub-e2e && whoami')
      setExecOut(
        `exit=${res.exit_code}\nstdout:\n${res.stdout}\nstderr:\n${res.stderr}`,
      )
    } catch (e) {
      setExecOut(e instanceof Error ? e.message : String(e))
    }
  }

  const tsDevices = tsStatus?.devices ?? []
  const newOnline = tsDevices.filter((d) => d.online && !d.already_added).length
  const newAll = tsDevices.filter((d) => !d.already_added).length

  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Machines</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Register hosts by IP/hostname, or pull from your Tailscale tailnet.
            Open the{' '}
            <Link className="underline" to="/workspace">
              Workspace
            </Link>{' '}
            for terminals.
          </p>
        </div>
        <Button
          type="button"
          variant="outline"
          onClick={() => void openTailscale()}
          className="shrink-0"
        >
          <Network className="size-4" />
          Import from Tailscale
        </Button>
      </div>

      {tsOpen ? (
        <div className="rounded-lg border border-border bg-card p-4">
          <div className="mb-3 flex items-center justify-between gap-2">
            <h2 className="text-sm font-semibold">Tailscale import</h2>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => setTsOpen(false)}
            >
              Close
            </Button>
          </div>

          {tsLoading ? (
            <p className="text-sm text-muted-foreground">Loading devices…</p>
          ) : !tsStatus?.configured ? (
            <div className="space-y-2 text-sm text-muted-foreground">
              <p>
                Not configured. On the <strong>api</strong> service set:
              </p>
              <pre className="overflow-x-auto rounded-md border border-border bg-muted/40 p-3 text-xs">
                {`TAILSCALE_API_KEY=tskey-api-...
# optional, default is "-" (your default tailnet)
TAILSCALE_TAILNET=-`}
              </pre>
              <p>
                Create an API access token at Tailscale Admin → Settings →
                Keys. Redeploy/restart api after setting env.
              </p>
            </div>
          ) : (
            <div className="space-y-4">
              <p className="text-sm text-muted-foreground">
                Found {tsDevices.length} authorized device(s). New to add:{' '}
                {tsOnlineOnly ? newOnline : newAll}
                {tsOnlineOnly ? ' (online only)' : ''}. Already registered
                addresses are skipped. Same SSH user/password applies to all
                imports — edit credentials later by re-registering if needed.
              </p>

              {tsDevices.length > 0 ? (
                <ul className="max-h-48 overflow-auto rounded-md border border-border text-sm">
                  {tsDevices.map((d) => (
                    <li
                      key={d.preferred_address}
                      className="flex items-center justify-between gap-2 border-b border-border px-3 py-2 last:border-0"
                    >
                      <div className="min-w-0">
                        <div className="truncate font-medium">{d.name}</div>
                        <div className="font-mono text-xs text-muted-foreground">
                          {d.preferred_address}
                          {d.os ? ` · ${d.os}` : ''}
                        </div>
                      </div>
                      <span className="shrink-0 text-xs text-muted-foreground">
                        {d.already_added
                          ? 'added'
                          : d.online
                            ? 'online'
                            : 'offline'}
                      </span>
                    </li>
                  ))}
                </ul>
              ) : (
                <p className="text-sm text-muted-foreground">
                  No authorized devices with addresses.
                </p>
              )}

              <div className="grid gap-3 sm:grid-cols-3">
                <Field label="SSH user" value={tsUser} onChange={setTsUser} />
                <Field
                  label="SSH password"
                  value={tsPassword}
                  onChange={setTsPassword}
                  type="password"
                  required={false}
                />
                <Field
                  label="SSH port"
                  value={String(tsPort)}
                  onChange={(v) => setTsPort(Number(v) || 22)}
                />
              </div>

              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={tsOnlineOnly}
                  onChange={(e) => setTsOnlineOnly(e.target.checked)}
                  className="size-4 rounded border-input"
                />
                Only import devices seen online recently
              </label>

              <div className="flex flex-wrap items-center gap-2">
                <Button
                  type="button"
                  disabled={tsBusy || (tsOnlineOnly ? newOnline : newAll) === 0}
                  onClick={() => void onImportTailscale()}
                >
                  <Network className="size-4" />
                  {tsBusy
                    ? 'Importing…'
                    : `Add ${tsOnlineOnly ? newOnline : newAll} machine(s)`}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={tsLoading || tsBusy}
                  onClick={() => void openTailscale()}
                >
                  Refresh list
                </Button>
              </div>

              {tsMsg ? (
                <p className="text-sm text-foreground" role="status">
                  {tsMsg}
                </p>
              ) : null}
            </div>
          )}
        </div>
      ) : null}

      <form
        onSubmit={(e) => void onCreate(e)}
        className="grid gap-3 rounded-lg border border-border bg-card p-4 sm:grid-cols-2"
      >
        <h2 className="sm:col-span-2 text-sm font-semibold">New device</h2>
        <Field label="Name" value={name} onChange={setName} />
        <Field
          label="Address (IP / hostname)"
          value={address}
          onChange={setAddress}
        />
        <Field
          label="SSH port"
          value={String(port)}
          onChange={(v) => setPort(Number(v) || 22)}
        />
        <Field label="SSH user" value={sshUser} onChange={setSshUser} />
        <Field
          label="SSH password"
          value={sshPassword}
          onChange={setSshPassword}
          type="password"
        />
        <div className="sm:col-span-2">
          <Button type="submit" disabled={busy}>
            <Plus className="size-4" />
            {busy ? 'Registering…' : 'Register machine'}
          </Button>
        </div>
      </form>

      {error ? (
        <p className="text-sm text-destructive" role="alert">
          {error}
        </p>
      ) : null}

      <div className="rounded-lg border border-amber-500/30 bg-amber-500/5 px-4 py-3 text-sm">
        <p className="font-medium text-foreground">
          Long sessions &amp; mobile browsers
        </p>
        <ul className="mt-2 list-disc space-y-1 pl-5 text-muted-foreground">
          <li>
            Agent Hub already keeps the browser WebSocket and SSH link alive
            (pings ~every 25s) and <strong>auto-reconnects</strong> after phone
            sleep / network flap. Shell state lives in <code>tmux</code> on the
            host — reconnect resumes the same session.
          </li>
          <li>
            JWT sessions default to <code>forever</code> (
            <code>JWT_ACCESS_TTL</code>). No re-login just because a terminal
            stayed open.
          </li>
          <li>
            <strong>If a host still drops idle SSH:</strong> on that machine
            (sshd), ensure keepalives are allowed, e.g. in{' '}
            <code>/etc/ssh/sshd_config</code>:{' '}
            <code>ClientAliveInterval 30</code>,{' '}
            <code>ClientAliveCountMax 240</code> (or higher), then{' '}
            <code>systemctl reload sshd</code>. Without this, some hardened
            hosts kill quiet sessions even when our client pings.
          </li>
          <li>
            <strong>Coolify / reverse proxy:</strong> domain must point at the{' '}
            <code>web</code> service (port 80). Edge idle timeout should be
            ≥ a few minutes (WS pings keep it warm). If Traefik/Coolify has a
            short custom timeout, raise it — Agent Hub cannot override the edge
            from inside the container.
          </li>
        </ul>
      </div>

      <div className="rounded-lg border border-border">
        <div className="border-b border-border px-4 py-2 text-sm font-medium">
          Registered machines {loading ? '(loading…)' : `(${machines.length})`}
        </div>
        {machines.length === 0 && !loading ? (
          <p className="p-4 text-sm text-muted-foreground">
            No machines yet. Import from Tailscale, or for Compose demo use
            address <code>ssh-target</code>, port <code>27343</code>, user{' '}
            <code>root</code>, password <code>targetpass</code>.
          </p>
        ) : (
          <ul className="divide-y divide-border">
            {machines.map((m) => (
              <li
                key={m.id}
                className="flex flex-col gap-2 px-4 py-3 sm:flex-row sm:items-center sm:justify-between"
              >
                <div className="min-w-0">
                  <div className="font-medium">{m.name}</div>
                  <div className="font-mono text-xs text-muted-foreground">
                    {m.ssh_user}@{m.address}:{m.port}
                  </div>
                  <p className="mt-1 text-[11px] leading-snug text-muted-foreground">
                    Host tip: if this box drops idle shells, set sshd{' '}
                    <code className="text-[10px]">ClientAliveInterval 30</code>{' '}
                    + high <code className="text-[10px]">ClientAliveCountMax</code>{' '}
                    (and install <code className="text-[10px]">tmux</code>).
                  </p>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => void onProbe(m.id)}
                  >
                    Probe SSH
                  </Button>
                  <Button variant="default" size="sm" asChild>
                    <Link to={`/workspace/${m.id}`}>
                      <LayoutPanelLeft className="size-4" />
                      Open workspace
                    </Link>
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => void onDelete(m.id, m.name)}
                    aria-label={`Delete machine ${m.name}`}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>

      {execOut ? (
        <pre className="overflow-auto rounded-md border border-border bg-muted/40 p-3 text-xs">
          {execOut}
        </pre>
      ) : null}
    </div>
  )
}

function Field({
  label,
  value,
  onChange,
  type = 'text',
  required = true,
}: {
  label: string
  value: string
  onChange: (v: string) => void
  type?: string
  required?: boolean
}) {
  return (
    <label className="flex flex-col gap-1 text-sm">
      <span className="text-muted-foreground">{label}</span>
      <input
        type={type}
        className="h-9 rounded-md border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        required={required}
      />
    </label>
  )
}
