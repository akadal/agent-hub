import {
  useCallback,
  useEffect,
  useId,
  useRef,
  useState,
  type FormEvent,
  type KeyboardEvent,
} from 'react'
import { ChevronDown, ChevronRight, Send } from 'lucide-react'

import { Button } from '@/components/ui/button'
import type { StreamBlock } from '@/lib/stream-blocks'
import { cn } from '@/lib/utils'

/** Chronological feed entry for the stream view. */
export type ChatFeedItem =
  | { type: 'user'; id: string; text: string }
  | { type: 'block'; id: string; block: StreamBlock }

type Props = {
  /** Ordered top→bottom history (user turns interleaved with stream blocks). */
  items: ChatFeedItem[]
  status?: string
  active: boolean
  disabled?: boolean
  onSend: (text: string) => void
  className?: string
}

/**
 * Minimal chat/stream feed for terminal output — light surface, collapsible
 * thinking regions, bottom composer. Does not own the PTY connection.
 */
export function TerminalChatView({
  items,
  status,
  active,
  disabled,
  onSend,
  className,
}: Props) {
  const listRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)
  const [draft, setDraft] = useState('')
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({})
  const stickBottom = useRef(true)
  const baseId = useId()

  // Auto-scroll when near bottom and content grows.
  useEffect(() => {
    const el = listRef.current
    if (!el || !stickBottom.current) return
    el.scrollTop = el.scrollHeight
  }, [items])

  useEffect(() => {
    if (active) {
      const t = window.setTimeout(() => inputRef.current?.focus(), 40)
      return () => window.clearTimeout(t)
    }
  }, [active])

  const onScroll = useCallback(() => {
    const el = listRef.current
    if (!el) return
    const dist = el.scrollHeight - el.scrollTop - el.clientHeight
    stickBottom.current = dist < 80
  }, [])

  const submit = useCallback(() => {
    const text = draft.replace(/\s+$/g, '')
    if (!text || disabled) return
    onSend(text)
    setDraft('')
    stickBottom.current = true
    requestAnimationFrame(() => {
      const el = listRef.current
      if (el) el.scrollTop = el.scrollHeight
    })
  }, [draft, disabled, onSend])

  const onForm = (e: FormEvent) => {
    e.preventDefault()
    submit()
  }

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      submit()
    }
  }

  const toggleCollapse = (id: string, defaultCollapsed: boolean) => {
    setCollapsed((prev) => {
      const current = prev[id] ?? defaultCollapsed
      return { ...prev, [id]: !current }
    })
  }

  const empty = items.length === 0

  return (
    <div
      className={cn(
        'flex h-full min-h-0 w-full flex-col bg-background text-foreground',
        className,
      )}
    >
      <div
        ref={listRef}
        onScroll={onScroll}
        className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-3 py-4 sm:px-5"
        role="log"
        aria-live="polite"
        aria-relevant="additions"
        aria-label="Terminal stream"
      >
        <div className="mx-auto flex w-full max-w-2xl flex-col gap-3">
          {empty ? (
            <div className="flex flex-col items-center gap-2 py-12 text-center text-sm text-muted-foreground">
              <p className="max-w-xs leading-relaxed">
                Stream view — terminal output appears as a readable feed.
                Type below to send input; slash commands stay visible.
              </p>
              {status ? (
                <p className="font-mono text-[11px] text-muted-foreground/80">
                  {status}
                </p>
              ) : null}
            </div>
          ) : null}

          {items.map((item) => {
            if (item.type === 'user') {
              return (
                <div key={item.id} className="flex justify-end">
                  <div
                    className={cn(
                      'max-w-[92%] rounded-2xl rounded-br-md bg-primary px-3.5 py-2 text-sm',
                      'text-primary-foreground shadow-sm',
                    )}
                  >
                    <pre className="whitespace-pre-wrap break-words font-sans leading-relaxed">
                      {item.text}
                    </pre>
                  </div>
                </div>
              )
            }

            const b = item.block
            const key = item.id

            if (b.kind === 'thinking' && b.collapsible) {
              const isCollapsed = collapsed[key] ?? true
              const panelId = `${baseId}-${key}`
              return (
                <div
                  key={key}
                  className="rounded-xl border border-border/80 bg-muted/40"
                >
                  <button
                    type="button"
                    className={cn(
                      'flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-xs font-medium',
                      'text-muted-foreground touch-manipulation transition-colors hover:bg-muted/60',
                    )}
                    aria-expanded={!isCollapsed}
                    aria-controls={panelId}
                    onClick={() => toggleCollapse(key, true)}
                  >
                    {isCollapsed ? (
                      <ChevronRight className="size-3.5 shrink-0" />
                    ) : (
                      <ChevronDown className="size-3.5 shrink-0" />
                    )}
                    <span>Thinking</span>
                    {isCollapsed ? (
                      <span className="truncate font-normal opacity-70">
                        · {b.text.slice(0, 48)}
                        {b.text.length > 48 ? '…' : ''}
                      </span>
                    ) : null}
                  </button>
                  {!isCollapsed ? (
                    <div
                      id={panelId}
                      className="border-t border-border/60 px-3 py-2"
                    >
                      <pre className="whitespace-pre-wrap break-words font-mono text-[12px] leading-relaxed text-muted-foreground sm:text-[13px]">
                        {b.text}
                      </pre>
                    </div>
                  ) : null}
                </div>
              )
            }

            if (b.kind === 'command') {
              return (
                <div
                  key={key}
                  className="rounded-xl border border-dashed border-border bg-card px-3.5 py-2"
                >
                  <div className="mb-0.5 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
                    Command
                  </div>
                  <pre className="whitespace-pre-wrap break-words font-mono text-[13px] leading-relaxed text-foreground">
                    {b.text}
                  </pre>
                </div>
              )
            }

            if (b.kind === 'system') {
              return (
                <div
                  key={key}
                  className="px-1 text-center text-[11px] text-muted-foreground"
                >
                  {b.text}
                </div>
              )
            }

            return (
              <div
                key={key}
                className="rounded-2xl rounded-bl-md border border-border/70 bg-card px-3.5 py-2.5 shadow-sm"
              >
                <pre className="whitespace-pre-wrap break-words font-mono text-[12.5px] leading-relaxed text-foreground sm:text-[13px]">
                  {b.text}
                </pre>
              </div>
            )
          })}
        </div>
      </div>

      <form
        onSubmit={onForm}
        className={cn(
          'shrink-0 border-t border-border bg-background/95 px-3 py-2.5 backdrop-blur',
          'supports-[backdrop-filter]:bg-background/90',
          'pb-[max(0.625rem,env(safe-area-inset-bottom))]',
        )}
      >
        <div className="mx-auto flex w-full max-w-2xl items-end gap-2">
          <label className="sr-only" htmlFor={`${baseId}-composer`}>
            Terminal input
          </label>
          <textarea
            id={`${baseId}-composer`}
            ref={inputRef}
            rows={1}
            value={draft}
            disabled={disabled}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={onKeyDown}
            placeholder="Send to terminal…  (/ commands work)"
            className={cn(
              'max-h-32 min-h-11 flex-1 resize-none rounded-xl border border-input',
              'bg-muted/30 px-3 py-2.5 text-base leading-snug text-foreground',
              'placeholder:text-muted-foreground/70',
              'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
              'disabled:opacity-50 sm:min-h-10 sm:text-sm',
            )}
          />
          <Button
            type="submit"
            size="icon"
            disabled={disabled || !draft.trim()}
            className="size-11 shrink-0 touch-manipulation rounded-xl sm:size-10"
            aria-label="Send"
            title="Send (Enter)"
          >
            <Send className="size-4" />
          </Button>
        </div>
      </form>
    </div>
  )
}
