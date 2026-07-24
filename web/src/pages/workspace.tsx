import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Terminal as XTerm } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import {
  Monitor,
  Plus,
  Terminal as TerminalIcon,
  Trash2,
  X,
} from 'lucide-react'
import '@xterm/xterm/css/xterm.css'

import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import {
  closeTerminal,
  createTerminal,
  listMachines,
  listTerminals,
  sessionWsUrl,
  type Machine,
  type TerminalSession,
} from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { cn } from '@/lib/utils'

/**
 * Session-centric workspace: machine → N sessions.
 * Terminal panes stay mounted while switching tabs so SSH + scrollback
 * are preserved (only visibility changes). Dispose on explicit Close.
 */
export function WorkspacePage() {
  const { machineId: routeMachineId } = useParams<{ machineId?: string }>()
  const [search, setSearch] = useSearchParams()
  const navigate = useNavigate()
  const { token } = useAuth()

  const [machines, setMachines] = useState<Machine[]>([])
  const [sessions, setSessions] = useState<TerminalSession[]>([])
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  /** Per-session connection status for the active tab header. */
  const [statusById, setStatusById] = useState<Record<string, string>>({})
  /**
   * Session terminals that stay alive in the DOM (xterm + WS).
   * Switching selection only toggles visibility — does not remount.
   */
  const [mountedSessionIds, setMountedSessionIds] = useState<string[]>([])

  const selectedMachineId = routeMachineId || machines[0]?.id || ''
  const selectedSessionId = search.get('session') || ''

  const selectedMachine = useMemo(
    () => machines.find((m) => m.id === selectedMachineId),
    [machines, selectedMachineId],
  )
  const selectedSession = useMemo(
    () => sessions.find((s) => s.id === selectedSessionId),
    [sessions, selectedSessionId],
  )

  const refreshMachines = useCallback(async () => {
    if (!token) return
    setMachines(await listMachines(token))
  }, [token])

  const refreshSessions = useCallback(async () => {
    if (!token || !selectedMachineId) {
      setSessions([])
      return
    }
    setSessions(await listTerminals(token, selectedMachineId))
  }, [token, selectedMachineId])

  useEffect(() => {
    void (async () => {
      try {
        setError(null)
        await refreshMachines()
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e))
      }
    })()
  }, [refreshMachines])

  useEffect(() => {
    void (async () => {
      try {
        setError(null)
        await refreshSessions()
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e))
      }
    })()
  }, [refreshSessions])

  useEffect(() => {
    if (!routeMachineId && machines[0]) {
      navigate(`/workspace/${machines[0].id}`, { replace: true })
    }
  }, [routeMachineId, machines, navigate])

  // Auto-select first session; keep selection valid after closes
  useEffect(() => {
    if (!selectedSessionId && sessions[0]) {
      setSearch({ session: sessions[0].id }, { replace: true })
      return
    }
    if (
      selectedSessionId &&
      sessions.length > 0 &&
      !sessions.some((s) => s.id === selectedSessionId)
    ) {
      if (sessions[0]) setSearch({ session: sessions[0].id }, { replace: true })
      else setSearch({}, { replace: true })
    }
  }, [sessions, selectedSessionId, setSearch])

  // Ensure the selected session is mounted (starts/keeps its live pane)
  useEffect(() => {
    if (!selectedSessionId) return
    setMountedSessionIds((prev) =>
      prev.includes(selectedSessionId) ? prev : [...prev, selectedSessionId],
    )
  }, [selectedSessionId])

  const mountSession = useCallback((id: string) => {
    setMountedSessionIds((prev) => (prev.includes(id) ? prev : [...prev, id]))
  }, [])

  const unmountSession = useCallback((id: string) => {
    setMountedSessionIds((prev) => prev.filter((x) => x !== id))
    setStatusById((prev) => {
      const next = { ...prev }
      delete next[id]
      return next
    })
  }, [])

  async function onNewSession() {
    if (!token || !selectedMachineId) return
    setBusy(true)
    setError(null)
    try {
      const name = window.prompt(
        'Session name',
        `Session ${sessions.length + 1}`,
      )
      if (name === null) return
      const t = await createTerminal(
        token,
        selectedMachineId,
        name.trim() || undefined,
      )
      await refreshSessions()
      mountSession(t.id)
      setSearch({ session: t.id })
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  async function onCloseSession(id: string) {
    if (!token) return
    // Tear down live pane first so WS/SSH close, then delete server record
    unmountSession(id)
    try {
      await closeTerminal(token, id)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
    await refreshSessions()
  }

  function selectMachine(id: string) {
    navigate(`/workspace/${id}`)
  }

  function selectSession(id: string) {
    mountSession(id)
    setSearch({ session: id })
  }

  const activeStatus =
    (selectedSessionId && statusById[selectedSessionId]) || 'idle'

  return (
    <div className="flex h-[calc(100vh-3.5rem-2rem)] min-h-[420px] flex-col gap-0 overflow-hidden rounded-xl border border-border bg-card shadow-sm md:flex-row">
      <aside className="flex w-full shrink-0 flex-col border-b border-border md:w-72 md:border-b-0 md:border-r">
        <div className="flex items-center justify-between gap-2 px-3 py-3">
          <div className="flex items-center gap-2 text-sm font-semibold">
            <Monitor className="size-4" />
            Workspace
          </div>
          <Button
            size="sm"
            variant="outline"
            disabled={!selectedMachineId || busy}
            onClick={() => void onNewSession()}
            title="New terminal session"
          >
            <Plus className="size-4" />
            New
          </Button>
        </div>
        <Separator />

        <div className="px-3 py-2 text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
          Machines
        </div>
        <ScrollArea className="max-h-36 px-2 md:max-h-40">
          {machines.length === 0 ? (
            <p className="px-2 py-2 text-xs text-muted-foreground">
              No machines. Register one under Machines.
            </p>
          ) : (
            <ul className="flex flex-col gap-0.5 pb-2">
              {machines.map((m) => (
                <li key={m.id}>
                  <button
                    type="button"
                    onClick={() => selectMachine(m.id)}
                    className={cn(
                      'flex w-full flex-col rounded-md px-2 py-1.5 text-left text-sm transition-colors',
                      m.id === selectedMachineId
                        ? 'bg-sidebar-accent text-sidebar-accent-foreground'
                        : 'hover:bg-muted/60',
                    )}
                  >
                    <span className="font-medium leading-tight">{m.name}</span>
                    <span className="font-mono text-[10px] text-muted-foreground">
                      {m.ssh_user}@{m.address}:{m.port}
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </ScrollArea>

        <Separator />
        <div className="px-3 py-2 text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
          Sessions
          {selectedMachine ? (
            <span className="normal-case tracking-normal text-muted-foreground/80">
              {' '}
              · {selectedMachine.name}
            </span>
          ) : null}
        </div>
        <ScrollArea className="min-h-0 flex-1 px-2">
          {sessions.length === 0 ? (
            <p className="px-2 py-3 text-xs text-muted-foreground">
              No sessions yet. Create one per task; switching tabs keeps each
              shell alive.
            </p>
          ) : (
            <ul className="flex flex-col gap-0.5 pb-3">
              {sessions.map((s) => {
                const live = mountedSessionIds.includes(s.id)
                return (
                  <li key={s.id} className="group flex items-stretch gap-0.5">
                    <button
                      type="button"
                      onClick={() => selectSession(s.id)}
                      className={cn(
                        'flex min-w-0 flex-1 items-center gap-2 rounded-md px-2 py-2 text-left text-sm transition-colors',
                        s.id === selectedSessionId
                          ? 'bg-primary text-primary-foreground'
                          : 'hover:bg-muted/60',
                      )}
                    >
                      <TerminalIcon className="size-3.5 shrink-0 opacity-80" />
                      <span className="truncate font-medium">{s.name}</span>
                      {live ? (
                        <span
                          className={cn(
                            'ml-auto size-1.5 shrink-0 rounded-full',
                            s.id === selectedSessionId
                              ? 'bg-primary-foreground/80'
                              : 'bg-emerald-500',
                          )}
                          title="Live shell attached"
                        />
                      ) : null}
                    </button>
                    <button
                      type="button"
                      className="rounded-md px-1.5 text-muted-foreground opacity-0 transition-opacity hover:bg-destructive/10 hover:text-destructive group-hover:opacity-100"
                      title="Close session"
                      onClick={() => void onCloseSession(s.id)}
                    >
                      <X className="size-3.5" />
                    </button>
                  </li>
                )
              })}
            </ul>
          )}
        </ScrollArea>
      </aside>

      <section className="flex min-w-0 flex-1 flex-col">
        <header className="flex flex-wrap items-center justify-between gap-2 border-b border-border px-4 py-2">
          <div className="min-w-0">
            <div className="truncate text-sm font-semibold">
              {selectedSession?.name ?? 'No session selected'}
            </div>
            <div className="truncate text-xs text-muted-foreground">
              {selectedMachine
                ? `${selectedMachine.name} · ${activeStatus}`
                : 'Select a machine'}
            </div>
          </div>
          <div className="flex gap-2">
            <Button
              size="sm"
              variant="outline"
              disabled={!selectedMachineId || busy}
              onClick={() => void onNewSession()}
            >
              <Plus className="size-4" />
              New session
            </Button>
            {selectedSession ? (
              <Button
                size="sm"
                variant="ghost"
                onClick={() => void onCloseSession(selectedSession.id)}
              >
                <Trash2 className="size-4" />
                Close
              </Button>
            ) : null}
          </div>
        </header>

        {error ? (
          <div className="border-b border-destructive/30 bg-destructive/10 px-4 py-2 text-sm text-destructive">
            {error}
          </div>
        ) : null}

        <div className="relative min-h-0 flex-1 bg-black p-2">
          {token && mountedSessionIds.length > 0 ? (
            mountedSessionIds.map((id) => (
              <div
                key={id}
                className={cn(
                  'absolute inset-2',
                  id === selectedSessionId
                    ? 'z-10 visible'
                    : 'z-0 invisible pointer-events-none',
                )}
                aria-hidden={id !== selectedSessionId}
              >
                <SessionTerminal
                  sessionId={id}
                  token={token}
                  active={id === selectedSessionId}
                  onStatus={(status) =>
                    setStatusById((prev) =>
                      prev[id] === status ? prev : { ...prev, [id]: status },
                    )
                  }
                />
              </div>
            ))
          ) : (
            <div className="flex h-full flex-col items-center justify-center gap-2 p-6 text-center text-sm text-neutral-400">
              <TerminalIcon className="size-8 opacity-50" />
              <p>
                Create a session for each task. Switching sessions keeps the
                shell and scrollback — only Close tears it down.
              </p>
            </div>
          )}
        </div>
      </section>
    </div>
  )
}

function SessionTerminal({
  sessionId,
  token,
  active,
  onStatus,
}: {
  sessionId: string
  token: string
  active: boolean
  onStatus: (s: string) => void
}) {
  const containerRef = useRef<HTMLDivElement>(null)
  const termRef = useRef<XTerm | null>(null)
  const fitRef = useRef<FitAddon | null>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const statusRef = useRef('idle')
  const onStatusRef = useRef(onStatus)
  onStatusRef.current = onStatus

  const setStatus = useCallback((s: string) => {
    statusRef.current = s
    onStatusRef.current(s)
  }, [])

  // Connect once per mount; stay alive while this component is mounted.
  useEffect(() => {
    if (!containerRef.current) return

    const term = new XTerm({
      cursorBlink: true,
      fontSize: 14,
      fontFamily:
        'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
      theme: {
        background: '#0a0a0a',
        foreground: '#e5e5e5',
        cursor: '#e5e5e5',
      },
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(containerRef.current)
    fit.fit()
    termRef.current = term
    fitRef.current = fit

    setStatus('connecting…')
    const ws = new WebSocket(sessionWsUrl(sessionId, token))
    wsRef.current = ws

    ws.onopen = () => {
      setStatus('connected')
      ws.send(
        JSON.stringify({
          type: 'resize',
          cols: term.cols,
          rows: term.rows,
        }),
      )
    }
    ws.onerror = () => setStatus('error')
    ws.onclose = () => {
      setStatus('closed')
      term.writeln('\r\n\x1b[33m[session closed]\x1b[0m')
    }
    ws.onmessage = (ev) => {
      try {
        const msg = JSON.parse(String(ev.data)) as {
          type: string
          data?: string
          message?: string
        }
        if (msg.type === 'stdout' && msg.data) term.write(msg.data)
        else if (msg.type === 'error') {
          setStatus('error')
          term.writeln(`\r\n\x1b[31m${msg.message}\x1b[0m`)
        } else if (msg.type === 'ready') setStatus('ssh ready')
        // pong ignored
      } catch {
        term.write(String(ev.data))
      }
    }

    const dataDisp = term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'stdin', data }))
      }
    })

    // Application-level ping so proxies / idle paths do not kill the WS
    const pingTimer = window.setInterval(() => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'ping' }))
      }
    }, 25_000)

    const onResize = () => {
      if (!fitRef.current || !termRef.current) return
      fitRef.current.fit()
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(
          JSON.stringify({
            type: 'resize',
            cols: termRef.current.cols,
            rows: termRef.current.rows,
          }),
        )
      }
    }
    window.addEventListener('resize', onResize)

    return () => {
      window.clearInterval(pingTimer)
      window.removeEventListener('resize', onResize)
      dataDisp.dispose()
      ws.close()
      wsRef.current = null
      term.dispose()
      termRef.current = null
      fitRef.current = null
    }
    // Intentionally only sessionId + token: remount only when identity changes
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId, token])

  // On tab focus: refit + focus xterm; publish current status to header
  useEffect(() => {
    if (!active) return
    onStatusRef.current(statusRef.current)
    // layout after becoming visible (invisible panes have zero size)
    const id = requestAnimationFrame(() => {
      const fit = fitRef.current
      const term = termRef.current
      const ws = wsRef.current
      if (!fit || !term) return
      fit.fit()
      term.focus()
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(
          JSON.stringify({
            type: 'resize',
            cols: term.cols,
            rows: term.rows,
          }),
        )
      }
    })
    return () => cancelAnimationFrame(id)
  }, [active])

  return <div ref={containerRef} className="h-full w-full" />
}
