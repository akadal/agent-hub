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
 * Session-centric workspace inspired by multi-session agent UIs:
 * machine context → session list → focused terminal pane.
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
  const [connStatus, setConnStatus] = useState<string>('idle')

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
    const list = await listMachines(token)
    setMachines(list)
  }, [token])

  const refreshSessions = useCallback(async () => {
    if (!token || !selectedMachineId) {
      setSessions([])
      return
    }
    const list = await listTerminals(token, selectedMachineId)
    setSessions(list)
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

  // Keep URL machine in sync when none selected
  useEffect(() => {
    if (!routeMachineId && machines[0]) {
      navigate(`/workspace/${machines[0].id}`, { replace: true })
    }
  }, [routeMachineId, machines, navigate])

  // Auto-select first session if none in query
  useEffect(() => {
    if (!selectedSessionId && sessions[0]) {
      setSearch({ session: sessions[0].id }, { replace: true })
    }
    if (
      selectedSessionId &&
      sessions.length > 0 &&
      !sessions.some((s) => s.id === selectedSessionId)
    ) {
      // closed session — pick another
      if (sessions[0]) {
        setSearch({ session: sessions[0].id }, { replace: true })
      } else {
        setSearch({}, { replace: true })
      }
    }
  }, [sessions, selectedSessionId, setSearch])

  async function onNewSession() {
    if (!token || !selectedMachineId) return
    setBusy(true)
    setError(null)
    try {
      const name = window.prompt('Session name', `Session ${sessions.length + 1}`)
      if (name === null) return
      const t = await createTerminal(token, selectedMachineId, name.trim() || undefined)
      await refreshSessions()
      setSearch({ session: t.id })
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  async function onCloseSession(id: string) {
    if (!token) return
    await closeTerminal(token, id)
    await refreshSessions()
  }

  function selectMachine(id: string) {
    navigate(`/workspace/${id}`)
  }

  function selectSession(id: string) {
    setSearch({ session: id })
  }

  return (
    <div className="flex h-[calc(100vh-3.5rem-2rem)] min-h-[420px] flex-col gap-0 overflow-hidden rounded-xl border border-border bg-card shadow-sm md:flex-row">
      {/* Left rail: machines + sessions */}
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
              No sessions yet. Create one for each parallel task (build, debug,
              logs, …).
            </p>
          ) : (
            <ul className="flex flex-col gap-0.5 pb-3">
              {sessions.map((s) => (
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
              ))}
            </ul>
          )}
        </ScrollArea>
      </aside>

      {/* Main pane */}
      <section className="flex min-w-0 flex-1 flex-col">
        <header className="flex flex-wrap items-center justify-between gap-2 border-b border-border px-4 py-2">
          <div className="min-w-0">
            <div className="truncate text-sm font-semibold">
              {selectedSession?.name ?? 'No session selected'}
            </div>
            <div className="truncate text-xs text-muted-foreground">
              {selectedMachine
                ? `${selectedMachine.name} · ${connStatus}`
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
          {selectedSessionId && token ? (
            <SessionTerminal
              key={selectedSessionId}
              sessionId={selectedSessionId}
              token={token}
              onStatus={setConnStatus}
            />
          ) : (
            <div className="flex h-full flex-col items-center justify-center gap-2 p-6 text-center text-sm text-neutral-400">
              <TerminalIcon className="size-8 opacity-50" />
              <p>
                Create a session to start a dedicated SSH shell for a task.
                Switch sessions anytime — each stays in the list.
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
  onStatus,
}: {
  sessionId: string
  token: string
  onStatus: (s: string) => void
}) {
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!containerRef.current) return

    const term = new XTerm({
      cursorBlink: true,
      fontSize: 14,
      fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
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

    onStatus('connecting…')
    const ws = new WebSocket(sessionWsUrl(sessionId, token))
    ws.onopen = () => {
      onStatus('connected')
      ws.send(
        JSON.stringify({
          type: 'resize',
          cols: term.cols,
          rows: term.rows,
        }),
      )
    }
    ws.onerror = () => onStatus('error')
    ws.onclose = () => {
      onStatus('closed')
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
          onStatus('error')
          term.writeln(`\r\n\x1b[31m${msg.message}\x1b[0m`)
        } else if (msg.type === 'ready') onStatus('ssh ready')
      } catch {
        term.write(String(ev.data))
      }
    }

    const dataDisp = term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'stdin', data }))
      }
    })

    const onResize = () => {
      fit.fit()
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(
          JSON.stringify({
            type: 'resize',
            cols: term.cols,
            rows: term.rows,
          }),
        )
      }
    }
    window.addEventListener('resize', onResize)

    return () => {
      window.removeEventListener('resize', onResize)
      dataDisp.dispose()
      ws.close()
      term.dispose()
    }
  }, [sessionId, token, onStatus])

  return <div ref={containerRef} className="h-full w-full" />
}
