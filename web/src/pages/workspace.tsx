import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Terminal as XTerm } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import {
  Maximize2,
  Minimize2,
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
 *
 * Fullscreen is layout-only (fixed overlay) — never remounts xterm/WS.
 * On mobile, the overlay is pinned to visualViewport so soft keyboard /
 * URL-bar changes do not thrash fit() and reflow the PTY.
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
  /** Immersive terminal UI (mobile-first); pure CSS — keeps shells alive. */
  const [fullscreen, setFullscreen] = useState(false)
  /** Bumped on layout changes so active xterm refits without remount. */
  const [layoutEpoch, setLayoutEpoch] = useState(0)
  /** Per-session connection status for the active tab header. */
  const [statusById, setStatusById] = useState<Record<string, string>>({})
  /**
   * Session terminals that stay alive in the DOM (xterm + WS).
   * Switching selection only toggles visibility — does not remount.
   */
  const [mountedSessionIds, setMountedSessionIds] = useState<string[]>([])

  const shellRef = useRef<HTMLDivElement>(null)
  const scheduleLayoutFit = useCallback(() => {
    setLayoutEpoch((n) => n + 1)
  }, [])

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

  const toggleFullscreen = useCallback(() => {
    setFullscreen((v) => !v)
    // After CSS applies, panes need a fit pass (window resize may not fire).
    scheduleLayoutFit()
  }, [scheduleLayoutFit])

  // Escape exits immersive mode; lock body scroll while fullscreen.
  // Pin shell to visualViewport (iOS keyboard / chrome) without React re-renders.
  useEffect(() => {
    if (!fullscreen) {
      const el = shellRef.current
      if (el) {
        el.style.top = ''
        el.style.left = ''
        el.style.width = ''
        el.style.height = ''
      }
      return
    }

    const html = document.documentElement
    const body = document.body
    const prev = {
      bodyOverflow: body.style.overflow,
      bodyTouch: body.style.touchAction,
      htmlOverflow: html.style.overflow,
      htmlOverscroll: html.style.overscrollBehavior,
    }
    body.style.overflow = 'hidden'
    body.style.touchAction = 'none'
    html.style.overflow = 'hidden'
    html.style.overscrollBehavior = 'none'

    let fitTimer = 0
    let lastW = 0
    let lastH = 0

    const pinToVisualViewport = () => {
      const el = shellRef.current
      const vv = window.visualViewport
      if (!el) return

      if (vv) {
        el.style.top = `${Math.round(vv.offsetTop)}px`
        el.style.left = `${Math.round(vv.offsetLeft)}px`
        el.style.width = `${Math.round(vv.width)}px`
        el.style.height = `${Math.round(vv.height)}px`
      } else {
        el.style.top = '0'
        el.style.left = '0'
        el.style.width = '100%'
        el.style.height = '100dvh'
      }

      const w = vv?.width ?? window.innerWidth
      const h = vv?.height ?? window.innerHeight
      // Only refit when the *size* changes (ignore pure offset scroll from iOS).
      if (Math.abs(w - lastW) >= 2 || Math.abs(h - lastH) >= 2) {
        lastW = w
        lastH = h
        window.clearTimeout(fitTimer)
        fitTimer = window.setTimeout(() => scheduleLayoutFit(), 80)
      }
    }

    pinToVisualViewport()

    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault()
        setFullscreen(false)
        scheduleLayoutFit()
      }
    }

    window.addEventListener('keydown', onKey)
    window.addEventListener('resize', pinToVisualViewport)
    const vv = window.visualViewport
    // scroll: reposition only (offsetTop changes); size gate above avoids fit spam
    vv?.addEventListener('resize', pinToVisualViewport)
    vv?.addEventListener('scroll', pinToVisualViewport)

    return () => {
      window.clearTimeout(fitTimer)
      body.style.overflow = prev.bodyOverflow
      body.style.touchAction = prev.bodyTouch
      html.style.overflow = prev.htmlOverflow
      html.style.overscrollBehavior = prev.htmlOverscroll
      window.removeEventListener('keydown', onKey)
      window.removeEventListener('resize', pinToVisualViewport)
      vv?.removeEventListener('resize', pinToVisualViewport)
      vv?.removeEventListener('scroll', pinToVisualViewport)
    }
  }, [fullscreen, scheduleLayoutFit])

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
    <div
      ref={shellRef}
      className={cn(
        'flex w-full flex-col gap-0 overflow-hidden bg-card md:flex-row',
        fullscreen
          ? // Cover app shell; visualViewport pin applied via ref styles on mobile.
            'fixed z-50 min-h-0 rounded-none border-0 bg-background pt-[env(safe-area-inset-top)] pb-[env(safe-area-inset-bottom)]'
          : 'h-full min-h-[280px] flex-1 rounded-none border-0 shadow-none md:min-h-[420px] md:rounded-xl md:border md:border-border md:shadow-sm',
      )}
      // Non-JS fallback geometry when not yet pinned by the fullscreen effect.
      style={
        fullscreen
          ? { top: 0, left: 0, width: '100%', height: '100dvh' }
          : undefined
      }
    >
      {/* Machines / sessions sidebar — hidden in immersive mode */}
      <aside
        className={cn(
          'flex max-h-[40vh] w-full shrink-0 flex-col border-b border-border md:max-h-none md:w-72 md:border-b-0 md:border-r',
          fullscreen && 'hidden',
        )}
      >
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
        <ScrollArea className="max-h-28 px-2 md:max-h-40">
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
                      className="rounded-md px-1.5 text-muted-foreground opacity-0 transition-opacity hover:bg-destructive/10 hover:text-destructive group-hover:opacity-100 max-md:opacity-100"
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

      <section className="flex min-h-0 min-w-0 flex-1 flex-col">
        <header
          className={cn(
            'flex shrink-0 items-center justify-between gap-2 border-b border-border',
            fullscreen
              ? 'bg-card/95 px-2 py-1.5 backdrop-blur supports-[backdrop-filter]:bg-card/80'
              : 'flex-wrap px-3 py-2 md:px-4',
          )}
        >
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm font-semibold">
              {selectedSession?.name ?? 'No session selected'}
            </div>
            <div className="truncate text-xs text-muted-foreground">
              {selectedMachine
                ? `${selectedMachine.name} · ${activeStatus}`
                : 'Select a machine'}
              {fullscreen ? (
                <span className="hidden text-muted-foreground/70 sm:inline">
                  {' '}
                  · Esc to exit
                </span>
              ) : null}
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-1 sm:gap-2">
            <Button
              size={fullscreen ? 'icon' : 'sm'}
              variant={fullscreen ? 'secondary' : 'outline'}
              onClick={toggleFullscreen}
              title={fullscreen ? 'Exit fullscreen (Esc)' : 'Fullscreen terminal'}
              aria-label={fullscreen ? 'Exit fullscreen' : 'Enter fullscreen'}
              aria-pressed={fullscreen}
              className="touch-manipulation max-md:size-9"
            >
              {fullscreen ? (
                <Minimize2 className="size-4" />
              ) : (
                <>
                  <Maximize2 className="size-4" />
                  <span className="hidden sm:inline">Fullscreen</span>
                </>
              )}
            </Button>
            {!fullscreen ? (
              <>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={!selectedMachineId || busy}
                  onClick={() => void onNewSession()}
                  className="touch-manipulation"
                >
                  <Plus className="size-4" />
                  <span className="hidden sm:inline">New session</span>
                </Button>
                {selectedSession ? (
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => void onCloseSession(selectedSession.id)}
                    className="touch-manipulation"
                  >
                    <Trash2 className="size-4" />
                    <span className="hidden sm:inline">Close</span>
                  </Button>
                ) : null}
              </>
            ) : (
              <Button
                size="icon"
                variant="ghost"
                disabled={!selectedMachineId || busy}
                onClick={() => void onNewSession()}
                title="New session"
                aria-label="New session"
                className="size-9 touch-manipulation"
              >
                <Plus className="size-4" />
              </Button>
            )}
          </div>
        </header>

        {/* Immersive session strip — switch shells without leaving fullscreen */}
        {fullscreen && sessions.length > 0 ? (
          <div className="flex shrink-0 gap-1.5 overflow-x-auto border-b border-border bg-muted/30 px-2 py-1.5 [-ms-overflow-style:none] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
            {sessions.map((s) => {
              const live = mountedSessionIds.includes(s.id)
              const selected = s.id === selectedSessionId
              return (
                <button
                  key={s.id}
                  type="button"
                  onClick={() => selectSession(s.id)}
                  className={cn(
                    'flex max-w-[10rem] shrink-0 items-center gap-1.5 rounded-full px-3 py-1.5 text-xs font-medium touch-manipulation transition-colors',
                    selected
                      ? 'bg-primary text-primary-foreground'
                      : 'bg-background/80 text-foreground hover:bg-muted',
                  )}
                >
                  <span className="truncate">{s.name}</span>
                  {live ? (
                    <span
                      className={cn(
                        'size-1.5 shrink-0 rounded-full',
                        selected
                          ? 'bg-primary-foreground/80'
                          : 'bg-emerald-500',
                      )}
                    />
                  ) : null}
                </button>
              )
            })}
          </div>
        ) : null}

        {error ? (
          <div className="shrink-0 border-b border-destructive/30 bg-destructive/10 px-4 py-2 text-sm text-destructive">
            {error}
          </div>
        ) : null}

        <div className="relative min-h-0 flex-1 bg-black">
          {token && mountedSessionIds.length > 0 ? (
            mountedSessionIds.map((id) => (
              <div
                key={id}
                className={cn(
                  'absolute',
                  // Desktop: small inset when not immersive; mobile fills the pane.
                  fullscreen ? 'inset-0' : 'inset-0 md:inset-2',
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
                  layoutEpoch={layoutEpoch}
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
              <p className="text-xs text-neutral-500 md:hidden">
                Tip: use Fullscreen for a stable mobile shell (hides chrome).
              </p>
            </div>
          )}
        </div>
      </section>
    </div>
  )
}

function prefersCoarsePointer(): boolean {
  if (typeof window === 'undefined') return false
  return window.matchMedia('(pointer: coarse)').matches
}

function isNarrowViewport(): boolean {
  if (typeof window === 'undefined') return false
  return window.matchMedia('(max-width: 640px)').matches
}

function SessionTerminal({
  sessionId,
  token,
  active,
  layoutEpoch = 0,
  onStatus,
}: {
  sessionId: string
  token: string
  active: boolean
  /** Parent bumps this on fullscreen / layout changes to force fit. */
  layoutEpoch?: number
  onStatus: (s: string) => void
}) {
  const containerRef = useRef<HTMLDivElement>(null)
  const termRef = useRef<XTerm | null>(null)
  const fitRef = useRef<FitAddon | null>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const lastSizeRef = useRef({ cols: 0, rows: 0 })
  const fitTimerRef = useRef(0)
  const statusRef = useRef('idle')
  const onStatusRef = useRef(onStatus)
  const activeRef = useRef(active)
  onStatusRef.current = onStatus
  activeRef.current = active

  const setStatus = useCallback((s: string) => {
    statusRef.current = s
    onStatusRef.current(s)
  }, [])

  /**
   * Fit the terminal to its host and notify PTY only when cols/rows change.
   * Debounced by default — continuous visualViewport noise was causing
   * mobile fullscreen thrash (text redraw / “echo” look).
   */
  const fitAndNotify = useCallback((opts?: { immediate?: boolean }) => {
    const run = () => {
      const fit = fitRef.current
      const term = termRef.current
      const ws = wsRef.current
      const host = containerRef.current
      if (!fit || !term || !host) return
      // Hidden / zero-size panes: measuring them collapses the PTY.
      if (!activeRef.current) return
      if (host.clientWidth < 16 || host.clientHeight < 16) return

      try {
        fit.fit()
      } catch {
        return
      }

      const cols = term.cols
      const rows = term.rows
      if (cols < 2 || rows < 1) return
      if (
        cols === lastSizeRef.current.cols &&
        rows === lastSizeRef.current.rows
      ) {
        return
      }
      lastSizeRef.current = { cols, rows }

      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(
          JSON.stringify({
            type: 'resize',
            cols,
            rows,
          }),
        )
      }
    }

    window.clearTimeout(fitTimerRef.current)
    if (opts?.immediate) {
      run()
      return
    }
    fitTimerRef.current = window.setTimeout(run, 100)
  }, [])

  // Connect once per mount; stay alive while this component is mounted.
  useEffect(() => {
    if (!containerRef.current) return

    const mobile = prefersCoarsePointer() || isNarrowViewport()
    const term = new XTerm({
      cursorBlink: !mobile,
      fontSize: mobile ? 12 : 14,
      lineHeight: 1.15,
      fontFamily:
        'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
      theme: {
        background: '#0a0a0a',
        foreground: '#e5e5e5',
        cursor: '#e5e5e5',
      },
      scrollback: mobile ? 2000 : 5000,
      // Avoid animated scroll redraw jank on soft devices.
      smoothScrollDuration: 0,
      // Slightly roomier touch targets for selection; still monospaced.
      allowTransparency: false,
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(containerRef.current)
    fit.fit()
    lastSizeRef.current = { cols: term.cols, rows: term.rows }
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

    // Prefer ResizeObserver on the host over window/visualViewport spam.
    let ro: ResizeObserver | null = null
    if (typeof ResizeObserver !== 'undefined' && containerRef.current) {
      ro = new ResizeObserver(() => {
        if (activeRef.current) fitAndNotify()
      })
      ro.observe(containerRef.current)
    }

    const onWindowResize = () => {
      if (activeRef.current) fitAndNotify()
    }
    window.addEventListener('resize', onWindowResize)
    // visualViewport resize only (NOT scroll) — keyboard open/close.
    const vv = window.visualViewport
    vv?.addEventListener('resize', onWindowResize)

    return () => {
      window.clearInterval(pingTimer)
      window.clearTimeout(fitTimerRef.current)
      window.removeEventListener('resize', onWindowResize)
      vv?.removeEventListener('resize', onWindowResize)
      ro?.disconnect()
      dataDisp.dispose()
      ws.close()
      wsRef.current = null
      term.dispose()
      termRef.current = null
      fitRef.current = null
      lastSizeRef.current = { cols: 0, rows: 0 }
    }
    // Intentionally only sessionId + token: remount only when identity changes
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId, token])

  // Layout change (fullscreen / vv size): refit only — do NOT re-focus.
  // Re-focusing on every fit re-opens the soft keyboard and restarts the jump cycle.
  useEffect(() => {
    if (!active) return
    onStatusRef.current(statusRef.current)
    let id2 = 0
    const id1 = requestAnimationFrame(() => {
      id2 = requestAnimationFrame(() => {
        fitAndNotify({ immediate: true })
      })
    })
    return () => {
      cancelAnimationFrame(id1)
      cancelAnimationFrame(id2)
    }
  }, [active, layoutEpoch, fitAndNotify])

  // Focus once when this tab becomes active (user selected the session).
  useEffect(() => {
    if (!active) return
    const t = window.setTimeout(() => termRef.current?.focus(), 30)
    return () => window.clearTimeout(t)
  }, [active, sessionId])

  return (
    <div
      ref={containerRef}
      className="ah-xterm-host h-full w-full"
      // Stop page-level rubber-band; xterm viewport still scrolls inside.
      onTouchMove={(e) => {
        e.stopPropagation()
      }}
    />
  )
}
