import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { Link } from 'react-router'
import { LayoutPanelLeft, Plus, Trash2, Network } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  createMachine,
  deleteMachine,
  checkMachine,
  execOnMachine,
  getTailscaleStatus,
  importFromTailscale,
  listMachines,
  type Machine,
  type MachineCheck,
  type TailscaleStatus,
} from '@/lib/api'
import { useAuth } from '@/lib/auth'

/** A check is either in flight (with its start time) or a finished verdict. */
type CheckState = MachineCheck | { running: true; startedAt: number }

function isRunning(c: CheckState | undefined): c is { running: true; startedAt: number } {
  return !!c && 'running' in c
}

function elapsedSeconds(c: CheckState | undefined): number {
  return isRunning(c) ? Math.max(0, Math.round((Date.now() - c.startedAt) / 1000)) : 0
}

/**
 * True for a machine whose port 22 is answered by Tailscale SSH rather than the
 * host's own sshd — a tailnet address (100.64.0.0/10) on the default port.
 * Mirrors isTailscaleSSHTarget in the backend: Tailscale intercepts port 22
 * only, so the same host on 2222 is ordinary OpenSSH.
 *
 * Worth flagging because such a target authenticates by tailnet ACL and ignores
 * the stored password entirely — without this the Machines page shows a
 * password field that looks like it is doing the work, and a failing host sends
 * you to rotate a credential that was never consulted (D-008).
 */
function isTailscaleSSHTarget(m: Machine): boolean {
  const octets = m.address.split('.')
  if (octets.length !== 4) return false
  const [a, b] = [Number(octets[0]), Number(octets[1])]
  return a === 100 && b >= 64 && b <= 127 && m.port === 22
}

export function MachinesPage() {
  const { token } = useAuth()
  const [machines, setMachines] = useState<Machine[]>([])
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  // Empty, not seeded with the demo host: pre-filling a real password meant
  // every registration started with `targetpass` already in the field, and it
  // silently became the credential for anyone who only edited the address.
  const [name, setName] = useState('')
  const [address, setAddress] = useState('')
  const [port, setPort] = useState(22)
  const [sshUser, setSshUser] = useState('root')
  const [sshPassword, setSshPassword] = useState('')
  const [sshPrivateKey, setSshPrivateKey] = useState('')
  const [sshKeyPassphrase, setSshKeyPassphrase] = useState('')
  const [busy, setBusy] = useState(false)
  const [execOut, setExecOut] = useState<string | null>(null)
  // Per-machine preflight verdicts, keyed by machine id.
  const [checks, setChecks] = useState<Record<string, CheckState>>({})
  // Ticks while any check is in flight so the elapsed counter advances. A
  // stalled SSH handshake takes seconds to give up, and a button that shows
  // nothing for that long is indistinguishable from a broken one.
  const [, setTick] = useState(0)

  // Tailscale import panel
  const [tsOpen, setTsOpen] = useState(false)
  const [tsStatus, setTsStatus] = useState<TailscaleStatus | null>(null)
  const [tsLoading, setTsLoading] = useState(false)
  const [tsUser, setTsUser] = useState('root')
  const [tsPassword, setTsPassword] = useState('')
  const [tsKey, setTsKey] = useState('')
  const [tsKeyPassphrase, setTsKeyPassphrase] = useState('')
  const [tsPort, setTsPort] = useState(22)
  const [tsOnlineOnly, setTsOnlineOnly] = useState(true)
  const [tsBusy, setTsBusy] = useState(false)
  const [tsMsg, setTsMsg] = useState<string | null>(null)

  useEffect(() => {
    const running = Object.values(checks).some(isRunning)
    if (!running) return
    const id = window.setInterval(() => setTick((n) => n + 1), 250)
    return () => window.clearInterval(id)
  }, [checks])

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
        ssh_private_key: tsKey.trim() || undefined,
        ssh_key_passphrase: tsKeyPassphrase || undefined,
      })
      setTsMsg(res.message)
      setTsKey('')
      setTsKeyPassphrase('')
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
        ssh_private_key: sshPrivateKey.trim() || undefined,
        ssh_key_passphrase: sshKeyPassphrase || undefined,
      })
      // Clear the whole form: leaving one host's credential in the fields is how
      // the next machine silently inherits it.
      setName('')
      setAddress('')
      setSshPassword('')
      setSshPrivateKey('')
      setSshKeyPassphrase('')
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

  async function onCheck(id: string) {
    if (!token) return
    setChecks((c) => ({ ...c, [id]: { running: true, startedAt: Date.now() } }))
    try {
      const result = await checkMachine(token, id)
      setChecks((c) => ({ ...c, [id]: result }))
    } catch (e) {
      setChecks((c) => ({
        ...c,
        [id]: {
          ok: false,
          elapsed_ms: 0,
          failure: {
            kind: 'request_failed',
            message: e instanceof Error ? e.message : String(e),
            retryable: true,
          },
        },
      }))
    }
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

              <PrivateKeyField
                id="ts-private-key"
                label="SSH private key (optional — applied to every imported device)"
                value={tsKey}
                onChange={setTsKey}
                passphrase={tsKeyPassphrase}
                onPassphraseChange={setTsKeyPassphrase}
                help="Tailnet fleets are usually keyed alike. Without a key, importing hosts that set PasswordAuthentication no registers machines you cannot open."
              />

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
        <Field label="Name" value={name} onChange={setName} placeholder="build-box" />
        <Field
          label="Address (IP / hostname)"
          value={address}
          onChange={setAddress}
          placeholder="10.0.0.5 or host.example"
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
          // A key-authenticated host has no password to give, and demanding one
          // here would block exactly the hardened hosts keys exist for.
          required={!sshPrivateKey.trim()}
          placeholder={sshPrivateKey.trim() ? 'not used — key takes precedence' : ''}
        />
        <div className="sm:col-span-2">
          <PrivateKeyField
            id="ssh-private-key"
            label="SSH private key (optional — used instead of the password)"
            value={sshPrivateKey}
            onChange={setSshPrivateKey}
            passphrase={sshKeyPassphrase}
            onPassphraseChange={setSshKeyPassphrase}
            help="Required for hosts with PasswordAuthentication no. Paste the whole PEM block, BEGIN and END lines included."
          />
        </div>
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
                    {/* Which credential the bridge will actually offer. The key
                        itself never leaves the API — only this flag does. */}
                    <span className="ml-2 rounded border border-border px-1.5 py-0.5 font-sans text-[10px] uppercase tracking-wide">
                      {m.has_private_key ? 'key' : 'password'}
                    </span>
                  </div>
                  {m.host_key_fingerprint ? (
                    <div
                      className="truncate font-mono text-[10px] text-muted-foreground"
                      title={`Host key pinned on first connect. Compare with: ssh-keyscan -p ${m.port} ${m.address}`}
                    >
                      host key {m.host_key_fingerprint}
                    </div>
                  ) : null}
                  {isTailscaleSSHTarget(m) ? (
                    <p className="mt-1 text-[11px] leading-snug text-amber-700 dark:text-amber-500">
                      Tailscale SSH target — port 22 on a tailnet address is
                      answered by Tailscale, which authenticates by tailnet ACL
                      and <strong>ignores the stored credential</strong>. If this
                      host fails, fix the ACL (grant the hub node{' '}
                      <code className="text-[10px]">action: accept</code>) rather
                      than the password. Using the host&apos;s own sshd on another
                      port avoids this entirely.
                    </p>
                  ) : null}
                  <p className="mt-1 text-[11px] leading-snug text-muted-foreground">
                    Host tip: if this box drops idle shells, set sshd{' '}
                    <code className="text-[10px]">ClientAliveInterval 30</code>{' '}
                    + high <code className="text-[10px]">ClientAliveCountMax</code>{' '}
                    (and install <code className="text-[10px]">tmux</code>).
                  </p>
                </div>
                {checks[m.id] ? <CheckVerdict result={checks[m.id]} /> : null}
                <div className="flex flex-wrap gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => void onCheck(m.id)}
                    disabled={isRunning(checks[m.id])}
                  >
                    {isRunning(checks[m.id])
                      ? `Testing… ${elapsedSeconds(checks[m.id])}s`
                      : 'Test connection'}
                  </Button>
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
  placeholder,
}: {
  label: string
  value: string
  onChange: (v: string) => void
  type?: string
  required?: boolean
  placeholder?: string
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
        placeholder={placeholder}
      />
    </label>
  )
}

/**
 * PEM key input plus the passphrase field it only needs once a key is present.
 * Shared by "New device" and the Tailscale import so both paths can register a
 * host that refuses passwords.
 */
function PrivateKeyField({
  id,
  label,
  value,
  onChange,
  passphrase,
  onPassphraseChange,
  help,
}: {
  id: string
  label: string
  value: string
  onChange: (v: string) => void
  passphrase: string
  onPassphraseChange: (v: string) => void
  help: string
}) {
  return (
    <div className="grid gap-1.5">
      <label htmlFor={id} className="text-xs font-medium text-muted-foreground">
        {label}
      </label>
      <textarea
        id={id}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        rows={4}
        spellCheck={false}
        autoComplete="off"
        placeholder={
          '-----BEGIN OPENSSH PRIVATE KEY-----\n…\n-----END OPENSSH PRIVATE KEY-----'
        }
        className="w-full rounded-md border border-border bg-background px-3 py-2 font-mono text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring"
      />
      <p className="text-xs text-muted-foreground">{help}</p>
      {value.trim() ? (
        <div className="mt-1 max-w-xs">
          <Field
            label="Key passphrase (if encrypted)"
            value={passphrase}
            onChange={onPassphraseChange}
            type="password"
            required={false}
          />
        </div>
      ) : null}
    </div>
  )
}

/**
 * Renders a preflight verdict. On failure it shows the classified cause and the
 * concrete fix — the point of the whole exercise is that a broken machine
 * explains itself here instead of in a black terminal twenty seconds later.
 */
function CheckVerdict({ result }: { result: CheckState }) {
  if (isRunning(result)) {
    const secs = elapsedSeconds(result)
    return (
      <p className="mt-2 text-xs text-muted-foreground">
        Testing connection… {secs}s
        {secs >= 3
          ? ' — a handshake this slow usually means the remote accepted the connection but never finished authenticating.'
          : ''}
      </p>
    )
  }
  if (result.ok) {
    return (
      <p className="mt-2 text-xs text-emerald-600 dark:text-emerald-400">
        Reachable — SSH authenticated in {result.elapsed_ms} ms.
      </p>
    )
  }
  const f = result.failure
  return (
    <div
      role="alert"
      className="mt-2 space-y-1 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs"
    >
      <p className="font-medium text-destructive">
        {f?.message ?? 'Connection failed'}
        {f?.kind ? (
          <span className="ml-1 font-mono font-normal opacity-70">
            [{f.kind}]
          </span>
        ) : null}
      </p>
      {f?.approval_url ? (
        <p>
          <a
            href={f.approval_url}
            target="_blank"
            rel="noreferrer noopener"
            className="underline"
          >
            Approve this login
          </a>
        </p>
      ) : null}
      {f?.hint ? <p className="text-muted-foreground">{f.hint}</p> : null}
      {f && !f.retryable ? (
        <p className="text-muted-foreground">
          Retrying will not help — apply the fix above first.
        </p>
      ) : null}
    </div>
  )
}
