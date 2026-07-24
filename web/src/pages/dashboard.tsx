import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import {
  Activity,
  ArrowRight,
  Clock,
  LayoutPanelLeft,
  Loader2,
  Plus,
  RefreshCw,
  Server,
  Terminal as TerminalIcon,
  Wifi,
  WifiOff,
} from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  API_BASE,
  createTerminal,
  execOnMachine,
  getTailscaleStatus,
  listAllTerminals,
  listMachines,
  type Machine,
  type TailscaleStatus,
  type TerminalSession,
} from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { cn } from '@/lib/utils'

type HealthResponse = {
  status: string
  service: string
}

type ProbeState = 'idle' | 'checking' | 'ok' | 'fail'

type MachineRow = Machine & {
  openSessions: number
  totalSessions: number
  probe: ProbeState
  probeDetail?: string
}

function relativeTime(iso: string): string {
  const t = new Date(iso).getTime()
  if (Number.isNaN(t)) return '—'
  const sec = Math.round((Date.now() - t) / 1000)
  if (sec < 10) return 'just now'
  if (sec < 60) return `${sec}s ago`
  const min = Math.round(sec / 60)
  if (min < 60) return `${min}m ago`
  const hr = Math.round(min / 60)
  if (hr < 48) return `${hr}h ago`
  const day = Math.round(hr / 24)
  return `${day}d ago`
}

function formatHost(m: Machine): string {
  return `${m.address}:${m.port}`
}

export function DashboardPage() {
  const { user, token } = useAuth()
  const navigate = useNavigate()
  const [health, setHealth] = useState<HealthResponse | null>(null)
  const [machines, setMachines] = useState<Machine[]>([])
  const [sessions, setSessions] = useState<TerminalSession[]>([])
  const [tsStatus, setTsStatus] = useState<TailscaleStatus | null>(null)
  const [probes, setProbes] = useState<Record<string, { state: ProbeState; detail?: string }>>({})
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [busyId, setBusyId] = useState<string | null>(null)
  const [lastRefreshed, setLastRefreshed] = useState<Date | null>(null)

  const load = useCallback(
    async (opts?: { quiet?: boolean }) => {
      if (!token) return
      if (opts?.quiet) setRefreshing(true)
      else setLoading(true)
      setError(null)
      try {
        const [hRes, mList, tList, ts] = await Promise.all([
          fetch(`${API_BASE}/health`)
            .then(async (r) => {
              if (!r.ok) throw new Error(`health HTTP ${r.status}`)
              return (await r.json()) as HealthResponse
            })
            .catch((e: unknown) => {
              throw e instanceof Error ? e : new Error(String(e))
            }),
          listMachines(token),
          listAllTerminals(token),
          getTailscaleStatus(token).catch(() => null),
        ])
        setHealth(hRes)
        setMachines(mList)
        const machineIds = new Set(mList.map((m) => m.id))
        // Scope sessions to machines the current user can see.
        setSessions(tList.filter((t) => machineIds.has(t.machine_id)))
        setTsStatus(ts)
        setLastRefreshed(new Date())
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e))
      } finally {
        setLoading(false)
        setRefreshing(false)
      }
    },
    [token],
  )

  useEffect(() => {
    void load()
  }, [load])

  // Soft auto-refresh every 30s while the tab is visible.
  useEffect(() => {
    const id = window.setInterval(() => {
      if (document.visibilityState === 'visible') void load({ quiet: true })
    }, 30_000)
    return () => window.clearInterval(id)
  }, [load])

  const openSessions = useMemo(
    () => sessions.filter((s) => s.status === 'open'),
    [sessions],
  )
  const closedSessions = useMemo(
    () => sessions.filter((s) => s.status !== 'open'),
    [sessions],
  )

  const sessionsByMachine = useMemo(() => {
    const map = new Map<string, TerminalSession[]>()
    for (const s of sessions) {
      const list = map.get(s.machine_id) ?? []
      list.push(s)
      map.set(s.machine_id, list)
    }
    return map
  }, [sessions])

  const machineRows: MachineRow[] = useMemo(() => {
    return machines
      .map((m) => {
        const list = sessionsByMachine.get(m.id) ?? []
        const open = list.filter((s) => s.status === 'open').length
        const p = probes[m.id]
        return {
          ...m,
          openSessions: open,
          totalSessions: list.length,
          probe: p?.state ?? 'idle',
          probeDetail: p?.detail,
        }
      })
      .sort((a, b) => a.name.localeCompare(b.name))
  }, [machines, sessionsByMachine, probes])

  const recentSessions = useMemo(() => {
    return [...sessions]
      .sort(
        (a, b) =>
          new Date(b.updated_at || b.created_at).getTime() -
          new Date(a.updated_at || a.created_at).getTime(),
      )
      .slice(0, 8)
  }, [sessions])

  const machineName = useCallback(
    (id: string) => machines.find((m) => m.id === id)?.name ?? id.slice(0, 8),
    [machines],
  )

  async function probeMachine(id: string) {
    if (!token) return
    setProbes((prev) => ({ ...prev, [id]: { state: 'checking' } }))
    try {
      const res = await execOnMachine(token, id, 'echo ok && hostname')
      if (res.exit_code === 0) {
        const host = res.stdout.trim().split('\n').pop() || 'reachable'
        setProbes((prev) => ({
          ...prev,
          [id]: { state: 'ok', detail: host },
        }))
      } else {
        setProbes((prev) => ({
          ...prev,
          [id]: {
            state: 'fail',
            detail: res.stderr.trim() || `exit ${res.exit_code}`,
          },
        }))
      }
    } catch (e) {
      setProbes((prev) => ({
        ...prev,
        [id]: {
          state: 'fail',
          detail: e instanceof Error ? e.message : String(e),
        },
      }))
    }
  }

  async function probeAll() {
    // Sequential to avoid hammering SSH targets.
    for (const m of machines) {
      await probeMachine(m.id)
    }
  }

  async function openNewSession(machineId: string) {
    if (!token) return
    setBusyId(machineId)
    setError(null)
    try {
      const t = await createTerminal(token, machineId)
      navigate(`/workspace/${machineId}?session=${t.id}`)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
      setBusyId(null)
    }
  }

  const tsOnline =
    tsStatus?.devices?.filter((d) => d.online).length ?? 0
  const tsTotal = tsStatus?.devices?.length ?? 0
  const tsNew =
    tsStatus?.devices?.filter((d) => !d.already_added).length ?? 0

  return (
    <div className="mx-auto flex max-w-6xl flex-col gap-6">
      {/* Header */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Dashboard</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Welcome back, <strong>{user?.username}</strong>
            {user?.role ? (
              <span className="text-muted-foreground"> · {user.role}</span>
            ) : null}
            . Overview of machines, sessions, and connectivity.
          </p>
          {lastRefreshed ? (
            <p className="mt-1 flex items-center gap-1.5 text-xs text-muted-foreground">
              <Clock className="size-3" />
              Updated {relativeTime(lastRefreshed.toISOString())}
              {refreshing ? (
                <Loader2 className="size-3 animate-spin" />
              ) : null}
            </p>
          ) : null}
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => void load({ quiet: true })}
            disabled={loading || refreshing}
          >
            <RefreshCw
              className={cn('size-4', refreshing && 'animate-spin')}
            />
            Refresh
          </Button>
          <Button variant="outline" size="sm" asChild>
            <Link to="/machines">
              <Server className="size-4" />
              Machines
            </Link>
          </Button>
          <Button size="sm" asChild>
            <Link to="/workspace">
              <LayoutPanelLeft className="size-4" />
              Workspace
            </Link>
          </Button>
        </div>
      </div>

      {error ? (
        <div className="rounded-md border border-destructive/40 bg-destructive/10 px-4 py-3 text-sm text-destructive">
          {error}
        </div>
      ) : null}

      {/* Stat cards */}
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard
          title="Machines"
          value={loading ? '—' : String(machines.length)}
          hint={
            machines.length === 0
              ? 'Register a host to start'
              : 'Registered hosts'
          }
          icon={<Server className="size-4" />}
          to="/machines"
        />
        <StatCard
          title="Open sessions"
          value={loading ? '—' : String(openSessions.length)}
          hint={`${closedSessions.length} closed · ${sessions.length} total`}
          icon={<TerminalIcon className="size-4" />}
          to="/workspace"
          accent="primary"
        />
        <StatCard
          title="API health"
          value={
            health?.status === 'ok'
              ? 'OK'
              : health
                ? health.status
                : loading
                  ? '…'
                  : 'Down'
          }
          hint={health?.service ?? 'agent-hub'}
          icon={<Activity className="size-4" />}
          tone={
            health?.status === 'ok'
              ? 'ok'
              : health
                ? 'warn'
                : loading
                  ? 'muted'
                  : 'bad'
          }
        />
        <StatCard
          title="Tailscale"
          value={
            tsStatus == null
              ? loading
                ? '…'
                : '—'
              : tsStatus.configured
                ? `${tsOnline}/${tsTotal}`
                : 'Off'
          }
          hint={
            tsStatus == null
              ? 'Status unavailable'
              : tsStatus.configured
                ? tsNew > 0
                  ? `${tsNew} not imported yet`
                  : 'devices online / total'
                : tsStatus.hint || 'API key not configured'
          }
          icon={<Wifi className="size-4" />}
          to="/machines"
          tone={
            tsStatus?.configured
              ? tsOnline > 0
                ? 'ok'
                : 'warn'
              : 'muted'
          }
        />
      </div>

      <div className="grid gap-6 lg:grid-cols-5">
        {/* Machines panel */}
        <section className="flex flex-col gap-3 lg:col-span-3">
          <div className="flex items-center justify-between gap-2">
            <h2 className="text-sm font-semibold tracking-tight">Machines</h2>
            <div className="flex gap-2">
              {machines.length > 0 ? (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => void probeAll()}
                  disabled={loading}
                >
                  <Wifi className="size-3.5" />
                  Probe all
                </Button>
              ) : null}
              <Button variant="outline" size="sm" asChild>
                <Link to="/machines">
                  <Plus className="size-3.5" />
                  Add
                </Link>
              </Button>
            </div>
          </div>

          {loading && machines.length === 0 ? (
            <EmptyPanel>Loading machines…</EmptyPanel>
          ) : machines.length === 0 ? (
            <EmptyPanel>
              <p className="mb-3">No machines yet.</p>
              <Button size="sm" asChild>
                <Link to="/machines">
                  <Plus className="size-4" />
                  Register first machine
                </Link>
              </Button>
            </EmptyPanel>
          ) : (
            <div className="overflow-hidden rounded-lg border border-border bg-card">
              <ul className="divide-y divide-border">
                {machineRows.map((m) => (
                  <li
                    key={m.id}
                    className="flex flex-col gap-2 px-4 py-3 sm:flex-row sm:items-center sm:justify-between"
                  >
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <Link
                          to={`/workspace/${m.id}`}
                          className="truncate font-medium hover:underline"
                        >
                          {m.name}
                        </Link>
                        <ProbeBadge state={m.probe} detail={m.probeDetail} />
                      </div>
                      <p className="mt-0.5 truncate font-mono text-xs text-muted-foreground">
                        {m.ssh_user}@{formatHost(m)}
                      </p>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {m.openSessions} open
                        {m.totalSessions !== m.openSessions
                          ? ` · ${m.totalSessions} total sessions`
                          : m.openSessions === 0
                            ? ' sessions'
                            : ' session' + (m.openSessions === 1 ? '' : 's')}
                        <span className="mx-1.5 text-border">·</span>
                        added {relativeTime(m.created_at)}
                      </p>
                    </div>
                    <div className="flex shrink-0 flex-wrap gap-1.5">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => void probeMachine(m.id)}
                        disabled={m.probe === 'checking'}
                        title="SSH reachability probe"
                      >
                        {m.probe === 'checking' ? (
                          <Loader2 className="size-3.5 animate-spin" />
                        ) : (
                          <Activity className="size-3.5" />
                        )}
                        Probe
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => void openNewSession(m.id)}
                        disabled={busyId === m.id}
                      >
                        {busyId === m.id ? (
                          <Loader2 className="size-3.5 animate-spin" />
                        ) : (
                          <Plus className="size-3.5" />
                        )}
                        Session
                      </Button>
                      <Button size="sm" asChild>
                        <Link to={`/workspace/${m.id}`}>
                          Open
                          <ArrowRight className="size-3.5" />
                        </Link>
                      </Button>
                    </div>
                  </li>
                ))}
              </ul>
            </div>
          )}
        </section>

        {/* Recent sessions + system */}
        <section className="flex flex-col gap-6 lg:col-span-2">
          <div className="flex flex-col gap-3">
            <div className="flex items-center justify-between gap-2">
              <h2 className="text-sm font-semibold tracking-tight">
                Recent sessions
              </h2>
              <Button variant="ghost" size="sm" asChild>
                <Link to="/workspace">
                  Workspace
                  <ArrowRight className="size-3.5" />
                </Link>
              </Button>
            </div>

            {loading && sessions.length === 0 ? (
              <EmptyPanel>Loading sessions…</EmptyPanel>
            ) : recentSessions.length === 0 ? (
              <EmptyPanel>
                No terminal sessions yet. Open a machine and create one.
              </EmptyPanel>
            ) : (
              <div className="overflow-hidden rounded-lg border border-border bg-card">
                <ul className="divide-y divide-border">
                  {recentSessions.map((s) => (
                    <li key={s.id}>
                      <Link
                        to={`/workspace/${s.machine_id}?session=${s.id}`}
                        className="flex items-start gap-3 px-4 py-3 transition-colors hover:bg-muted/50"
                      >
                        <div
                          className={cn(
                            'mt-1 size-2 shrink-0 rounded-full',
                            s.status === 'open'
                              ? 'bg-emerald-500'
                              : 'bg-muted-foreground/40',
                          )}
                          title={s.status}
                        />
                        <div className="min-w-0 flex-1">
                          <div className="flex items-center justify-between gap-2">
                            <span className="truncate text-sm font-medium">
                              {s.name}
                            </span>
                            <span className="shrink-0 text-[11px] text-muted-foreground">
                              {relativeTime(s.updated_at || s.created_at)}
                            </span>
                          </div>
                          <p className="mt-0.5 truncate text-xs text-muted-foreground">
                            {machineName(s.machine_id)}
                            <span className="mx-1">·</span>
                            <span
                              className={cn(
                                s.status === 'open'
                                  ? 'text-emerald-600 dark:text-emerald-400'
                                  : '',
                              )}
                            >
                              {s.status}
                            </span>
                            {s.remote_session ? (
                              <>
                                <span className="mx-1">·</span>
                                <span className="font-mono">
                                  {s.remote_session}
                                </span>
                              </>
                            ) : null}
                          </p>
                        </div>
                      </Link>
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </div>

          <div className="flex flex-col gap-3">
            <h2 className="text-sm font-semibold tracking-tight">System</h2>
            <div className="rounded-lg border border-border bg-card p-4 text-sm">
              <dl className="grid gap-3">
                <SystemRow
                  label="API"
                  value={
                    health?.status === 'ok' ? (
                      <span className="text-emerald-600 dark:text-emerald-400">
                        healthy
                      </span>
                    ) : (
                      <span className="text-destructive">
                        {health?.status ?? 'unreachable'}
                      </span>
                    )
                  }
                />
                <SystemRow
                  label="Service"
                  value={
                    <span className="font-mono text-xs">
                      {health?.service ?? '—'}
                    </span>
                  }
                />
                <SystemRow
                  label="Tailscale import"
                  value={
                    tsStatus?.configured ? (
                      <span>
                        configured · {tsOnline} online
                        {tsNew > 0 ? ` · ${tsNew} new` : ''}
                      </span>
                    ) : (
                      <span className="text-muted-foreground">
                        not configured
                      </span>
                    )
                  }
                />
                <SystemRow
                  label="Your role"
                  value={<span className="capitalize">{user?.role}</span>}
                />
              </dl>
              <div className="mt-4 flex flex-wrap gap-2 border-t border-border pt-4">
                <Button variant="outline" size="sm" asChild>
                  <Link to="/machines">Manage machines</Link>
                </Button>
                <Button size="sm" asChild>
                  <Link to="/workspace">
                    <LayoutPanelLeft className="size-3.5" />
                    Open workspace
                  </Link>
                </Button>
              </div>
            </div>
          </div>
        </section>
      </div>
    </div>
  )
}

function StatCard({
  title,
  value,
  hint,
  icon,
  to,
  tone = 'muted',
  accent,
}: {
  title: string
  value: string
  hint: string
  icon: ReactNode
  to?: string
  tone?: 'muted' | 'ok' | 'warn' | 'bad'
  accent?: 'primary'
}) {
  const body = (
    <div
      className={cn(
        'rounded-lg border border-border bg-card p-4 text-card-foreground shadow-sm transition-colors',
        to && 'hover:bg-muted/40',
        accent === 'primary' && 'ring-1 ring-primary/10',
      )}
    >
      <div className="mb-2 flex items-center justify-between gap-2 text-sm font-medium text-muted-foreground">
        <span className="flex items-center gap-2">
          {icon}
          {title}
        </span>
        {to ? <ArrowRight className="size-3.5 opacity-50" /> : null}
      </div>
      <p
        className={cn(
          'text-2xl font-semibold tracking-tight',
          tone === 'ok' && 'text-emerald-600 dark:text-emerald-400',
          tone === 'warn' && 'text-amber-600 dark:text-amber-400',
          tone === 'bad' && 'text-destructive',
        )}
      >
        {value}
      </p>
      <p className="mt-1 truncate text-xs text-muted-foreground">{hint}</p>
    </div>
  )
  if (to) return <Link to={to}>{body}</Link>
  return body
}

function ProbeBadge({
  state,
  detail,
}: {
  state: ProbeState
  detail?: string
}) {
  if (state === 'idle') return null
  if (state === 'checking') {
    return (
      <span className="inline-flex items-center gap-1 rounded-full bg-muted px-2 py-0.5 text-[10px] font-medium text-muted-foreground">
        <Loader2 className="size-2.5 animate-spin" />
        probing
      </span>
    )
  }
  if (state === 'ok') {
    return (
      <span
        className="inline-flex items-center gap-1 rounded-full bg-emerald-500/10 px-2 py-0.5 text-[10px] font-medium text-emerald-700 dark:text-emerald-400"
        title={detail}
      >
        <Wifi className="size-2.5" />
        reachable
        {detail ? (
          <span className="max-w-[8rem] truncate font-mono opacity-80">
            {detail}
          </span>
        ) : null}
      </span>
    )
  }
  return (
    <span
      className="inline-flex items-center gap-1 rounded-full bg-destructive/10 px-2 py-0.5 text-[10px] font-medium text-destructive"
      title={detail}
    >
      <WifiOff className="size-2.5" />
      unreachable
    </span>
  )
}

function SystemRow({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-3">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="text-right">{value}</dd>
    </div>
  )
}

function EmptyPanel({ children }: { children: ReactNode }) {
  return (
    <div className="rounded-lg border border-dashed border-border bg-muted/20 px-4 py-8 text-center text-sm text-muted-foreground">
      {children}
    </div>
  )
}
