import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router'
import { Terminal as XTerm } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import {
  MessageSquareText,
  Maximize2,
  Minimize2,
  Monitor,
  PanelLeft,
  Plus,
  Terminal as TerminalIcon,
  Trash2,
  X,
} from 'lucide-react'
import '@xterm/xterm/css/xterm.css'

import {
  TerminalChatView,
  type ChatFeedItem,
} from '@/components/terminal-chat-view'
import { TerminalKeyBar } from '@/components/terminal-keybar'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import { Sheet, SheetContent, SheetTitle } from '@/components/ui/sheet'
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
import {
  createStreamFeed,
  feedAddUser,
  feedPush,
  feedSnapshot,
  type StreamFeed,
} from '@/lib/stream-blocks'
import { controlSequence } from '@/lib/terminal-keys'
import { useTheme } from '@/lib/theme'
import type { ResolvedTheme } from '@/lib/theme-pref'
import { terminalSurface, xtermTheme } from '@/lib/terminal-theme'
import {
  TERMINAL_FONT_MAX,
  TERMINAL_FONT_MIN,
  getTerminalFontSize,
  getTerminalViewMode,
  setTerminalFontSize,
  setTerminalViewMode,
  type TerminalViewMode,
} from '@/lib/terminal-view-pref'
import { cn } from '@/lib/utils'

/**
 * Session-centric workspace: machine → N sessions.
 * Terminal panes stay mounted while switching tabs so SSH + scrollback
 * are preserved (only visibility changes). Dispose on explicit Close.
 *
 * Fullscreen is layout-only (fixed overlay) — never remounts xterm/WS.
 * On mobile, the overlay is pinned to visualViewport so soft keyboard /
 * URL-bar changes do not thrash fit() and reflow the PTY.
 *
 * Phone layout: the machine/session picker is a sheet, not a pane. It used to
 * take a third of the screen above the shell, which left a terminal barely
 * ten lines tall on a handset.
 */
export function WorkspacePage() {
  const { machineId: routeMachineId } = useParams<{ machineId?: string }>()
  const [search, setSearch] = useSearchParams()
  const navigate = useNavigate()
  const { token } = useAuth()
  const { resolved: appTheme } = useTheme()

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
  /**
   * Global display mode (classic xterm vs readable stream). Shared by all
   * sessions; persisted in localStorage — not per-session.
   */
  const [viewMode, setViewMode] = useState<TerminalViewMode>(getTerminalViewMode)
  /** Phone: machines/sessions live in a sheet so the shell keeps the viewport. */
  const [pickerOpen, setPickerOpen] = useState(false)
  /**
   * Soft keys (Esc/Tab/Ctrl/arrows). On by default for touch input, where the
   * OS keyboard simply has no way to send them.
   */
  const [softKeys, setSoftKeys] = useState(prefersCoarsePointer)
  const [fontSize, setFontSizeState] = useState(() =>
    getTerminalFontSize(isNarrowViewport()),
  )

  const shellRef = useRef<HTMLDivElement>(null)
  const scheduleLayoutFit = useCallback(() => {
    setLayoutEpoch((n) => n + 1)
  }, [])

  const changeFontSize = useCallback(
    (delta: number) => {
      setFontSizeState((prev) => setTerminalFontSize(prev + delta))
      scheduleLayoutFit()
    },
    [scheduleLayoutFit],
  )

  const toggleViewMode = useCallback(() => {
    setViewMode((prev) => {
      const next: TerminalViewMode = prev === 'chat' ? 'classic' : 'chat'
      setTerminalViewMode(next)
      return next
    })
    // Classic needs a fit pass after becoming visible again.
    scheduleLayoutFit()
  }, [scheduleLayoutFit])

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
      setPickerOpen(false)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  async function onCloseSession(id: string) {
    if (!token) return
    const session = sessions.find((s) => s.id === id)
    const label = session?.name?.trim() || 'this session'
    if (
      !window.confirm(
        `Close “${label}”? The remote shell will be terminated and this cannot be undone.`,
      )
    ) {
      return
    }
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

  const selectSession = useCallback(
    (id: string) => {
      mountSession(id)
      setSearch({ session: id })
      setPickerOpen(false)
    },
    [mountSession, setSearch],
  )

  const activeStatus =
    (selectedSessionId && statusById[selectedSessionId]) || 'idle'

  const picker = (
    <WorkspacePicker
      machines={machines}
      sessions={sessions}
      selectedMachineId={selectedMachineId}
      selectedSessionId={selectedSessionId}
      selectedMachine={selectedMachine}
      mountedSessionIds={mountedSessionIds}
      busy={busy}
      fontSize={fontSize}
      softKeys={softKeys}
      onSelectMachine={selectMachine}
      onSelectSession={selectSession}
      onNewSession={() => void onNewSession()}
      onCloseSession={(id) => void onCloseSession(id)}
      onFontSize={changeFontSize}
      onToggleSoftKeys={() => setSoftKeys((v) => !v)}
    />
  )

  return (
    <div
      ref={shellRef}
      className={cn(
        'flex w-full flex-col gap-0 overflow-hidden bg-card md:flex-row',
        fullscreen
          ? // Cover app shell; visualViewport pin applied via ref styles on mobile.
            'fixed z-50 min-h-0 rounded-none border-0 bg-background pt-[env(safe-area-inset-top)]'
          : 'h-full min-h-[280px] flex-1 rounded-none border-0 shadow-none md:min-h-[420px] md:rounded-xl md:border md:border-border md:shadow-sm',
      )}
      // Non-JS fallback geometry when not yet pinned by the fullscreen effect.
      style={
        fullscreen
          ? { top: 0, left: 0, width: '100%', height: '100dvh' }
          : undefined
      }
    >
      {/* Desktop: permanent picker column. Immersive mode hides it. */}
      <aside
        className={cn(
          'hidden w-72 shrink-0 flex-col border-r border-border md:flex',
          fullscreen && 'md:hidden',
        )}
      >
        {picker}
      </aside>

      {/* Phone: same picker, as an overlay — the shell keeps the whole screen. */}
      <Sheet open={pickerOpen} onOpenChange={setPickerOpen}>
        <SheetContent side="left" className="w-[19rem] bg-card p-0 md:hidden">
          <SheetTitle className="sr-only">Machines and sessions</SheetTitle>
          {/* Keep the header clear of the sheet's own close button. */}
          <div className="flex h-full min-h-0 flex-col [&_[data-picker-head]]:pr-10">
            {picker}
          </div>
        </SheetContent>
      </Sheet>

      <section className="flex min-h-0 min-w-0 flex-1 flex-col">
        <header
          className={cn(
            'flex shrink-0 items-center gap-1.5 border-b border-border sm:gap-2',
            fullscreen
              ? 'bg-card/95 px-2 py-1.5 backdrop-blur supports-[backdrop-filter]:bg-card/80'
              : 'px-2 py-1.5 md:px-4 md:py-2',
          )}
        >
          <Button
            size="icon"
            variant="ghost"
            onClick={() => setPickerOpen(true)}
            className="size-9 shrink-0 touch-manipulation md:hidden"
            title="Machines & sessions"
            aria-label="Machines and sessions"
          >
            <PanelLeft className="size-4" />
          </Button>

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
              variant={viewMode === 'chat' ? 'secondary' : 'outline'}
              onClick={toggleViewMode}
              title={
                viewMode === 'chat'
                  ? 'Switch to the classic terminal'
                  : 'Switch to the readable stream view'
              }
              aria-label={
                viewMode === 'chat'
                  ? 'Switch to the classic terminal'
                  : 'Switch to the readable stream view'
              }
              aria-pressed={viewMode === 'chat'}
              className="shrink-0 touch-manipulation max-md:size-9"
            >
              {viewMode === 'chat' ? (
                <TerminalIcon className="size-4" />
              ) : (
                <MessageSquareText className="size-4" />
              )}
              {/* Labels only where the desktop layout starts — between sm and
                  md the header is still the phone one and they overflow it. */}
              <span className="hidden md:inline">
                {viewMode === 'chat' ? 'Classic' : 'Stream'}
              </span>
            </Button>
            <Button
              size={fullscreen ? 'icon' : 'sm'}
              variant={fullscreen ? 'secondary' : 'outline'}
              onClick={toggleFullscreen}
              title={fullscreen ? 'Exit fullscreen (Esc)' : 'Fullscreen terminal'}
              aria-label={fullscreen ? 'Exit fullscreen' : 'Enter fullscreen'}
              aria-pressed={fullscreen}
              className="shrink-0 touch-manipulation max-md:size-9"
            >
              {fullscreen ? (
                <Minimize2 className="size-4" />
              ) : (
                <>
                  <Maximize2 className="size-4" />
                  <span className="hidden md:inline">Fullscreen</span>
                </>
              )}
            </Button>
            {/* New / Close are one tap away in the strip and the picker on phones. */}
            {!fullscreen ? (
              <>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={!selectedMachineId || busy}
                  onClick={() => void onNewSession()}
                  className="hidden touch-manipulation md:inline-flex"
                >
                  <Plus className="size-4" />
                  <span className="hidden lg:inline">New session</span>
                </Button>
                {selectedSession ? (
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => void onCloseSession(selectedSession.id)}
                    className="hidden touch-manipulation md:inline-flex"
                  >
                    <Trash2 className="size-4" />
                    <span className="hidden lg:inline">Close</span>
                  </Button>
                ) : null}
              </>
            ) : null}
          </div>
        </header>

        {/*
          Session tabs. Always on phones — switching shells used to mean opening
          a picker that covered the terminal. Desktop keeps them for immersive
          mode, where the picker column is hidden.
        */}
        {sessions.length > 0 ? (
          <div
            className={cn(
              'flex shrink-0 gap-1.5 overflow-x-auto border-b border-border bg-muted/30 px-2 py-1.5',
              '[-ms-overflow-style:none] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden',
              fullscreen ? 'flex' : 'md:hidden',
            )}
          >
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
            <button
              type="button"
              onClick={() => void onNewSession()}
              disabled={!selectedMachineId || busy}
              title="New session"
              aria-label="New session"
              className="flex size-8 shrink-0 touch-manipulation items-center justify-center rounded-full bg-background/80 text-foreground transition-colors hover:bg-muted disabled:opacity-50"
            >
              <Plus className="size-4" />
            </button>
          </div>
        ) : null}

        {error ? (
          <div className="shrink-0 border-b border-destructive/30 bg-destructive/10 px-4 py-2 text-sm text-destructive">
            {error}
          </div>
        ) : null}

        <div
          className="relative min-h-0 flex-1"
          style={{
            background:
              viewMode === 'chat' ? undefined : terminalSurface(appTheme),
          }}
        >
          {token && mountedSessionIds.length > 0 ? (
            mountedSessionIds.map((id) => (
              <div
                key={id}
                className={cn(
                  'absolute',
                  // Desktop: small inset when not immersive; mobile fills the pane.
                  // Stream mode always edge-to-edge on the app surface.
                  viewMode === 'chat' || fullscreen
                    ? 'inset-0'
                    : 'inset-0 md:inset-2',
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
                  viewMode={viewMode}
                  theme={appTheme}
                  fontSize={fontSize}
                  softKeys={softKeys}
                  onStatus={(status) =>
                    setStatusById((prev) =>
                      prev[id] === status ? prev : { ...prev, [id]: status },
                    )
                  }
                />
              </div>
            ))
          ) : (
            <div className="flex h-full flex-col items-center justify-center gap-2 p-6 text-center text-sm text-muted-foreground">
              <TerminalIcon className="size-8 opacity-50" />
              <p>
                Create a session for each task. Switching sessions keeps the
                shell and scrollback — only Close tears it down.
              </p>
              <Button
                size="sm"
                variant="outline"
                disabled={!selectedMachineId || busy}
                onClick={() => void onNewSession()}
                className="mt-1 touch-manipulation"
              >
                <Plus className="size-4" />
                New session
              </Button>
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

/**
 * Machines, sessions and per-device terminal settings. Rendered twice — as the
 * desktop column and inside the phone sheet — so the two can never drift.
 */
function WorkspacePicker({
  machines,
  sessions,
  selectedMachineId,
  selectedSessionId,
  selectedMachine,
  mountedSessionIds,
  busy,
  fontSize,
  softKeys,
  onSelectMachine,
  onSelectSession,
  onNewSession,
  onCloseSession,
  onFontSize,
  onToggleSoftKeys,
}: {
  machines: Machine[]
  sessions: TerminalSession[]
  selectedMachineId: string
  selectedSessionId: string
  selectedMachine?: Machine
  mountedSessionIds: string[]
  busy: boolean
  fontSize: number
  softKeys: boolean
  onSelectMachine: (id: string) => void
  onSelectSession: (id: string) => void
  onNewSession: () => void
  onCloseSession: (id: string) => void
  onFontSize: (delta: number) => void
  onToggleSoftKeys: () => void
}) {
  return (
    <div className="flex h-full min-h-0 flex-col">
      <div
        data-picker-head
        className="flex items-center justify-between gap-2 px-3 py-3"
      >
        <div className="flex items-center gap-2 text-sm font-semibold">
          <Monitor className="size-4" />
          Workspace
        </div>
        <Button
          size="sm"
          variant="outline"
          disabled={!selectedMachineId || busy}
          onClick={onNewSession}
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
      <ScrollArea className="max-h-40 px-2">
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
                  onClick={() => onSelectMachine(m.id)}
                  className={cn(
                    'flex w-full flex-col rounded-md px-2 py-2 text-left text-sm transition-colors',
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
                    onClick={() => onSelectSession(s.id)}
                    className={cn(
                      'flex min-w-0 flex-1 items-center gap-2 rounded-md px-2 py-2.5 text-left text-sm transition-colors',
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
                    className="rounded-md px-2 text-muted-foreground opacity-0 transition-opacity hover:bg-destructive/10 hover:text-destructive group-hover:opacity-100 max-md:opacity-100"
                    title="Close session"
                    aria-label={`Close ${s.name}`}
                    onClick={() => onCloseSession(s.id)}
                  >
                    <X className="size-3.5" />
                  </button>
                </li>
              )
            })}
          </ul>
        )}
      </ScrollArea>

      <Separator />
      <div className="flex flex-col gap-2 p-3 pb-[max(0.75rem,env(safe-area-inset-bottom))]">
        <div className="flex items-center justify-between gap-2">
          <span className="text-xs text-muted-foreground">Text size</span>
          <div className="flex items-center gap-1">
            <Button
              size="icon"
              variant="outline"
              className="size-8 touch-manipulation"
              onClick={() => onFontSize(-1)}
              disabled={fontSize <= TERMINAL_FONT_MIN}
              title="Smaller terminal text"
              aria-label="Smaller terminal text"
            >
              <span className="text-xs">A−</span>
            </Button>
            <span className="w-8 text-center font-mono text-xs text-muted-foreground">
              {fontSize}
            </span>
            <Button
              size="icon"
              variant="outline"
              className="size-8 touch-manipulation"
              onClick={() => onFontSize(1)}
              disabled={fontSize >= TERMINAL_FONT_MAX}
              title="Larger terminal text"
              aria-label="Larger terminal text"
            >
              <span className="text-sm">A+</span>
            </Button>
          </div>
        </div>
        <label className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
          <span>Soft keys (Esc, Tab, Ctrl…)</span>
          <input
            type="checkbox"
            className="size-4 accent-primary"
            checked={softKeys}
            onChange={onToggleSoftKeys}
          />
        </label>
      </div>
    </div>
  )
}

/** Seed the stream feed from what xterm already has, so switching view keeps history. */
function seedFeedFromTerminal(feed: StreamFeed, term: XTerm, maxLines = 200) {
  const buf = term.buffer.active
  const end = buf.length
  const start = Math.max(0, end - maxLines)
  const lines: string[] = []
  for (let i = start; i < end; i++) {
    const line = buf.getLine(i)
    if (!line) continue
    const text = line.translateToString(true)
    // A long logical line occupies several rows; joining them back keeps the
    // reader from showing one command split across three "lines".
    if (line.isWrapped && lines.length > 0) lines[lines.length - 1] += text
    else lines.push(text)
  }
  const text = lines.join('\n').replace(/\n+$/, '')
  if (text.trim()) feedPush(feed, `${text}\n`)
}

function SessionTerminal({
  sessionId,
  token,
  active,
  layoutEpoch = 0,
  viewMode = 'classic',
  theme,
  fontSize,
  softKeys,
  onStatus,
}: {
  sessionId: string
  token: string
  active: boolean
  /** Parent bumps this on fullscreen / layout changes to force fit. */
  layoutEpoch?: number
  /** Global presentation mode — does not remount WS/xterm. */
  viewMode?: TerminalViewMode
  theme: ResolvedTheme
  fontSize: number
  softKeys: boolean
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
  const viewModeRef = useRef(viewMode)
  onStatusRef.current = onStatus
  activeRef.current = active
  viewModeRef.current = viewMode

  // Theme and font size are xterm *options*, not construction args: changing
  // them must never rebuild the terminal, or the shell and scrollback would go
  // with it. The mount effect reads the current value through these refs.
  const themeRef = useRef(theme)
  themeRef.current = theme
  const fontSizeRef = useRef(fontSize)
  fontSizeRef.current = fontSize

  /** Sticky Ctrl from the soft key bar — applies to the next key typed. */
  const [ctrlArmed, setCtrlArmed] = useState(false)
  const ctrlArmedRef = useRef(false)
  ctrlArmedRef.current = ctrlArmed

  /**
   * Stream feed lives in a ref and is only fed while stream view is on.
   * Classic — the default — pays nothing for PTY volume, which is what made
   * the first attempt at this view unusable.
   */
  const feedRef = useRef<StreamFeed | null>(null)
  if (!feedRef.current) feedRef.current = createStreamFeed()
  const feedTimerRef = useRef(0)
  const [feedGen, setFeedGen] = useState(0)

  const setStatus = useCallback((s: string) => {
    statusRef.current = s
    onStatusRef.current(s)
  }, [])

  /** Coalesce PTY bursts into at most ~8 React commits per second. */
  const scheduleFeedUi = useCallback(() => {
    if (viewModeRef.current !== 'chat') return
    if (feedTimerRef.current) return
    feedTimerRef.current = window.setTimeout(() => {
      feedTimerRef.current = 0
      setFeedGen(feedRef.current?.gen ?? 0)
    }, 120)
  }, [])

  const sendStdin = useCallback((data: string) => {
    const ws = wsRef.current
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ type: 'stdin', data }))
    }
  }, [])

  /** Soft key press: raw bytes straight to the PTY, and un-stick Ctrl. */
  const onSoftKey = useCallback(
    (data: string) => {
      sendStdin(data)
      setCtrlArmed(false)
      if (viewModeRef.current === 'classic') termRef.current?.focus()
    },
    [sendStdin],
  )

  const onToggleKeyboard = useCallback(() => {
    const term = termRef.current
    if (!term) return
    // xterm keeps focus in a hidden textarea; blurring it drops the OS keyboard.
    const helper = containerRef.current?.querySelector<HTMLTextAreaElement>(
      '.xterm-helper-textarea',
    )
    if (helper && document.activeElement === helper) helper.blur()
    else term.focus()
  }, [])

  const onChatSend = useCallback(
    (text: string) => {
      const feed = feedRef.current
      if (feed) {
        feedAddUser(feed, text)
        setFeedGen(feed.gen)
      }
      // Line-oriented submit; Enter for most shells / agent CLIs.
      sendStdin(text.endsWith('\n') || text.endsWith('\r') ? text : `${text}\r`)
    },
    [sendStdin],
  )

  /**
   * Fit the terminal to its host and notify PTY only when cols/rows change.
   * Debounced by default — continuous visualViewport noise was causing
   * mobile fullscreen thrash (text redraw / “echo” look).
   * Stream mode keeps a fixed cols/rows for the remote PTY.
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

      // Stream mode: host may be off-screen with zero size — use fixed geometry.
      if (viewModeRef.current === 'chat') {
        const cols = 100
        const rows = 32
        if (
          cols !== lastSizeRef.current.cols ||
          rows !== lastSizeRef.current.rows
        ) {
          lastSizeRef.current = { cols, rows }
          try {
            term.resize(cols, rows)
          } catch {
            /* ignore */
          }
          if (ws && ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({ type: 'resize', cols, rows }))
          }
        }
        return
      }

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

  // xterm mounts once; WebSocket auto-reconnects (mobile Safari/Chrome kill
  // idle sockets when the screen locks or the tab is backgrounded).
  // Server tmux session is durable — reconnect reattaches the same shell.
  useEffect(() => {
    if (!containerRef.current) return

    const mobile = prefersCoarsePointer() || isNarrowViewport()
    const term = new XTerm({
      cursorBlink: !mobile,
      fontSize: fontSizeRef.current,
      lineHeight: 1.15,
      fontFamily:
        'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
      theme: xtermTheme(themeRef.current),
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

    let disposed = false
    let socketGen = 0
    let currentWs: WebSocket | null = null
    let reconnectTimer = 0
    let reconnectAttempt = 0
    let hadReady = false
    /** True once we saw a server "error" for this socket (e.g. ssh open timeout). */
    let openFailed = false
    let pingTimer = 0
    let pokeTimer = 0
    let sawStdout = false
    let openWatchTimer = 0

    const clearReconnect = () => {
      window.clearTimeout(reconnectTimer)
      reconnectTimer = 0
    }

    const clearPing = () => {
      window.clearInterval(pingTimer)
      pingTimer = 0
    }

    const clearPoke = () => {
      window.clearTimeout(pokeTimer)
      pokeTimer = 0
    }

    const clearOpenWatch = () => {
      window.clearTimeout(openWatchTimer)
      openWatchTimer = 0
    }

    const sendAppPing = () => {
      const ws = currentWs
      if (ws && ws.readyState === WebSocket.OPEN) {
        try {
          ws.send(JSON.stringify({ type: 'ping' }))
        } catch {
          /* ignore */
        }
      }
    }

    /** Mirror into the stream feed only while that view is the one on screen. */
    const feedText = (data: string) => {
      if (viewModeRef.current !== 'chat') return
      const feed = feedRef.current
      if (!feed) return
      if (feedPush(feed, data)) scheduleFeedUi()
    }

    const write = (data: string) => {
      term.write(data)
      feedText(data)
    }

    const writeln = (line: string) => {
      term.writeln(line)
      feedText(`${line}\n`)
    }

    const connect = () => {
      if (disposed) return
      clearReconnect()
      clearPing()
      clearPoke()
      clearOpenWatch()
      sawStdout = false
      openFailed = false

      const gen = ++socketGen
      // Drop previous socket without triggering its reconnect path.
      if (currentWs) {
        const prev = currentWs
        currentWs = null
        prev.onopen = null
        prev.onclose = null
        prev.onerror = null
        prev.onmessage = null
        try {
          prev.close()
        } catch {
          /* ignore */
        }
      }

      setStatus(hadReady ? 'reconnecting…' : 'connecting…')
      // Fit before first frame so PTY size is sane (not 0×0).
      try {
        fit.fit()
      } catch {
        /* ignore */
      }
      const ws = new WebSocket(sessionWsUrl(sessionId, token))
      currentWs = ws
      wsRef.current = ws

      ws.onopen = () => {
        if (disposed || gen !== socketGen) return
        reconnectAttempt = 0
        // WS is up — SSH dial still in flight on the API. Do not say "connected"
        // (users read that as "shell ready" and stare at a black cursor).
        setStatus('opening ssh…')
        writeln('\x1b[90m[ws connected — opening remote shell…]\x1b[0m')
        try {
          ws.send(
            JSON.stringify({
              type: 'resize',
              cols: Math.max(term.cols, 2),
              rows: Math.max(term.rows, 1),
            }),
          )
        } catch {
          /* ignore */
        }
        // App-level ping as a second line of defence (proxies that ignore WS pings).
        // Mobile throttles timers when backgrounded; server protocol pings still run.
        pingTimer = window.setInterval(sendAppPing, 20_000)
        sendAppPing()
        // If the API never sends ready/error (hung SSH), surface it instead of
        // infinite black screen with a blinking cursor.
        openWatchTimer = window.setTimeout(() => {
          if (disposed || gen !== socketGen || hadReady || openFailed) return
          openFailed = true
          setStatus('error')
          writeln(
            '\x1b[31m[ssh open timed out — no shell from server. Check machine SSH/Tailscale, then New session.]\x1b[0m',
          )
          try {
            ws.close()
          } catch {
            /* ignore */
          }
        }, 30_000)
      }

      ws.onerror = () => {
        if (disposed || gen !== socketGen) return
        // onclose decides whether to reconnect.
        if (!hadReady) setStatus('error')
        else setStatus('reconnecting…')
      }

      ws.onclose = () => {
        if (disposed || gen !== socketGen) return
        clearPing()
        clearPoke()
        clearOpenWatch()
        wsRef.current = null
        if (currentWs === ws) currentWs = null

        // CRITICAL: do not auto-reconnect after a failed SSH open.
        // Reconnecting every few seconds piles hung Tailscale/SSH dials and
        // produces the "timeout / timeout / timeout" spam the user saw.
        if (!hadReady || openFailed) {
          setStatus('error')
          return
        }

        // Exponential backoff: 1s, 2s, 4s … cap 15s. Phone sleep often needs a few tries.
        const delay = Math.min(1000 * 2 ** reconnectAttempt, 15_000)
        reconnectAttempt += 1
        setStatus('reconnecting…')
        writeln(
          `\x1b[33m[connection lost — reconnecting in ${Math.round(delay / 1000)}s…]\x1b[0m`,
        )
        reconnectTimer = window.setTimeout(() => {
          if (!disposed && gen === socketGen) connect()
        }, delay)
      }

      ws.onmessage = (ev) => {
        if (disposed || gen !== socketGen) return
        try {
          const msg = JSON.parse(String(ev.data)) as {
            type: string
            data?: string
            message?: string
            /** Classified SSH open failure (see backend sshterm.Failure). */
            kind?: string
            hint?: string
            approval_url?: string
            retryable?: boolean
          }
          if (msg.type === 'stdout' && msg.data) {
            sawStdout = true
            clearPoke()
            write(msg.data)
          } else if (msg.type === 'error') {
            clearOpenWatch()
            clearPoke()
            openFailed = true
            setStatus('error')
            writeln(`\x1b[31m${msg.message ?? 'ssh error'}\x1b[0m`)
            // The server classifies why the open failed. Show the fix and any
            // approval link instead of leaving a bare timeout on a black screen.
            if (msg.kind) {
              writeln(`\x1b[90m[cause: ${msg.kind}]\x1b[0m`)
            }
            if (msg.approval_url) {
              writeln(`\x1b[36m[approve this login: ${msg.approval_url}]\x1b[0m`)
            }
            if (msg.hint) {
              writeln(`\x1b[33m${msg.hint}\x1b[0m`)
            }
            writeln(
              msg.retryable === false
                ? '\x1b[33m[auto-reconnect off — retrying cannot fix this; apply the fix above, then New session]\x1b[0m'
                : '\x1b[33m[stopped auto-reconnect — click the session again or New session to retry]\x1b[0m',
            )
          } else if (msg.type === 'status' && msg.message) {
            setStatus('opening ssh…')
            writeln(`\x1b[90m[${msg.message}]\x1b[0m`)
          } else if (msg.type === 'ready') {
            clearOpenWatch()
            openFailed = false
            const resume = hadReady
            hadReady = true
            setStatus('ssh ready')
            if (resume) {
              writeln('\x1b[32m[reconnected — same tmux session]\x1b[0m')
            } else {
              writeln(`\x1b[90m[${msg.message ?? 'ssh ready'}]\x1b[0m`)
            }
            // Some hosts (or empty tmux panes) open a PTY but never paint a
            // prompt. Nudge with Enter once so the user is not stuck on a
            // black screen with only a blinking cursor.
            clearPoke()
            pokeTimer = window.setTimeout(() => {
              if (disposed || gen !== socketGen || sawStdout) return
              const sock = currentWs
              if (sock && sock.readyState === WebSocket.OPEN) {
                writeln('\x1b[33m[no shell output yet — sending Enter…]\x1b[0m')
                try {
                  sock.send(JSON.stringify({ type: 'stdin', data: '\r' }))
                } catch {
                  /* ignore */
                }
              }
            }, 1500)
          }
          // pong ignored
        } catch {
          write(String(ev.data))
        }
      }
    }

    connect()

    const dataDisp = term.onData((data) => {
      const ws = currentWs
      if (!ws || ws.readyState !== WebSocket.OPEN) return
      let out = data
      // Sticky Ctrl from the soft key bar: the OS keyboard sends a plain
      // letter, we turn it into the control code the shell expects.
      if (ctrlArmedRef.current) {
        const ctrl = controlSequence(data)
        if (ctrl !== null) out = ctrl
        ctrlArmedRef.current = false
        setCtrlArmed(false)
      }
      ws.send(JSON.stringify({ type: 'stdin', data: out }))
    })

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

    // Mobile: screen unlock / tab focus / network back — reconnect immediately
    // instead of waiting out a long backoff while the socket is already dead.
    const wakeReconnect = () => {
      if (disposed) return
      const open =
        currentWs &&
        (currentWs.readyState === WebSocket.OPEN ||
          currentWs.readyState === WebSocket.CONNECTING)
      if (open) {
        sendAppPing()
        return
      }
      clearReconnect()
      reconnectAttempt = 0
      connect()
    }
    const onVisibility = () => {
      if (document.visibilityState === 'visible') wakeReconnect()
    }
    document.addEventListener('visibilitychange', onVisibility)
    window.addEventListener('online', wakeReconnect)
    window.addEventListener('pageshow', wakeReconnect)
    // iOS sometimes freezes the page; resume is a strong reconnect signal.
    document.addEventListener('resume', wakeReconnect as EventListener)

    return () => {
      disposed = true
      socketGen += 1 // invalidate in-flight handlers
      clearReconnect()
      clearPing()
      clearPoke()
      clearOpenWatch()
      window.clearTimeout(fitTimerRef.current)
      window.clearTimeout(feedTimerRef.current)
      feedTimerRef.current = 0
      window.removeEventListener('resize', onWindowResize)
      vv?.removeEventListener('resize', onWindowResize)
      document.removeEventListener('visibilitychange', onVisibility)
      window.removeEventListener('online', wakeReconnect)
      window.removeEventListener('pageshow', wakeReconnect)
      document.removeEventListener('resume', wakeReconnect as EventListener)
      ro?.disconnect()
      dataDisp.dispose()
      if (currentWs) {
        const ws = currentWs
        currentWs = null
        ws.onopen = null
        ws.onclose = null
        ws.onerror = null
        ws.onmessage = null
        try {
          ws.close()
        } catch {
          /* ignore */
        }
      }
      wsRef.current = null
      term.dispose()
      termRef.current = null
      fitRef.current = null
      lastSizeRef.current = { cols: 0, rows: 0 }
    }
    // Intentionally only sessionId + token: remount only when identity changes
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId, token])

  useEffect(() => {
    const term = termRef.current
    if (!term) return
    term.options.theme = xtermTheme(theme)
  }, [theme])

  useEffect(() => {
    const term = termRef.current
    if (!term) return
    term.options.fontSize = fontSize
    if (active) fitAndNotify({ immediate: true })
  }, [fontSize, active, fitAndNotify])

  // Layout change (fullscreen / vv size / view mode): refit only — do NOT re-focus.
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
  }, [active, layoutEpoch, viewMode, fitAndNotify])

  // Entering stream view: seed from what xterm already holds, so the feed is
  // not blank until the next command. Nothing is re-parsed after this.
  useEffect(() => {
    if (viewMode !== 'chat') return
    const feed = feedRef.current
    const term = termRef.current
    if (!feed || !term) return
    if (feed.blocks.length === 0 && !feed.open) {
      seedFeedFromTerminal(feed, term)
    }
    setFeedGen(feed.gen)
  }, [viewMode])

  // Focus classic xterm once when this tab becomes active (user selected the session).
  useEffect(() => {
    if (!active || viewMode !== 'classic') return
    const t = window.setTimeout(() => termRef.current?.focus(), 30)
    return () => window.clearTimeout(t)
  }, [active, sessionId, viewMode])

  const isChat = viewMode === 'chat'

  const chatItems = useMemo((): ChatFeedItem[] => {
    if (!isChat) return []
    const feed = feedRef.current
    if (!feed) return []
    void feedGen
    return feedSnapshot(feed).map((b) =>
      b.kind === 'user'
        ? ({ type: 'user', id: b.id, text: b.text } as const)
        : ({ type: 'block', id: b.id, block: b } as const),
    )
  }, [isChat, feedGen])

  return (
    <div className="relative flex h-full w-full flex-col">
      <div className="relative min-h-0 flex-1">
        {/*
          Keep xterm mounted always so scrollback + WS stay alive when toggling.
          Stream mode parks it off-screen (still receives writes).
        */}
        <div
          ref={containerRef}
          className={cn(
            'ah-xterm-host',
            isChat
              ? 'pointer-events-none absolute h-px w-px overflow-hidden opacity-0'
              : 'h-full w-full',
          )}
          aria-hidden={isChat}
          // Stop page-level rubber-band; xterm viewport still scrolls inside.
          onTouchMove={(e) => {
            if (!isChat) e.stopPropagation()
          }}
        />
        {isChat ? (
          <TerminalChatView
            className="absolute inset-0"
            items={chatItems}
            status={statusRef.current}
            active={active}
            onSend={onChatSend}
          />
        ) : null}
      </div>

      {softKeys && !isChat ? (
        <TerminalKeyBar
          onSend={onSoftKey}
          ctrlArmed={ctrlArmed}
          onToggleCtrl={() => setCtrlArmed((v) => !v)}
          onToggleKeyboard={onToggleKeyboard}
        />
      ) : null}
    </div>
  )
}
