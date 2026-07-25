import {
  memo,
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

export type ChatFeedItem =
  | { type: 'user'; id: string; text: string }
  | { type: 'block'; id: string; block: StreamBlock }

type Props = {
  items: ChatFeedItem[]
  status?: string
  active: boolean
  disabled?: boolean
  onSend: (text: string) => void
  className?: string
}

const FeedRow = memo(function FeedRow({
  item,
  baseId,
  collapsed,
  onToggle,
}: {
  item: ChatFeedItem
  baseId: string
  collapsed: Record<string, boolean>
  onToggle: (id: string) => void
}) {
  if (item.type === 'user') {
    return (
      <div
        className="flex justify-end py-1.5"
        style={{ contentVisibility: 'auto', containIntrinsicSize: '0 48px' }}
      >
        <div className="max-w-[88%] rounded-2xl bg-primary px-3.5 py-2 text-[15px] leading-snug text-primary-foreground">
          <pre className="whitespace-pre-wrap break-words font-sans">
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
        className="py-1"
        style={{ contentVisibility: 'auto', containIntrinsicSize: '0 36px' }}
      >
        <button
          type="button"
          className={cn(
            'flex w-full max-w-full items-center gap-1.5 rounded-lg py-1.5 text-left text-[12px]',
            'text-muted-foreground touch-manipulation hover:text-foreground',
          )}
          aria-expanded={!isCollapsed}
          aria-controls={panelId}
          onClick={() => onToggle(key)}
        >
          {isCollapsed ? (
            <ChevronRight className="size-3.5 shrink-0 opacity-70" />
          ) : (
            <ChevronDown className="size-3.5 shrink-0 opacity-70" />
          )}
          <span className="font-medium">Thinking</span>
          {isCollapsed ? (
            <span className="min-w-0 truncate font-normal text-muted-foreground/80">
              {b.text.replace(/\s+/g, ' ').slice(0, 64)}
              {b.text.length > 64 ? '…' : ''}
            </span>
          ) : null}
        </button>
        {!isCollapsed ? (
          <pre
            id={panelId}
            className="mt-1 whitespace-pre-wrap break-words border-l-2 border-border pl-3 font-mono text-[12px] leading-relaxed text-muted-foreground"
          >
            {b.text}
          </pre>
        ) : null}
      </div>
    )
  }

  if (b.kind === 'command') {
    return (
      <div
        className="py-1"
        style={{ contentVisibility: 'auto', containIntrinsicSize: '0 28px' }}
      >
        <pre className="whitespace-pre-wrap break-words font-mono text-[13px] leading-relaxed text-violet-700 dark:text-violet-300">
          {b.text}
        </pre>
      </div>
    )
  }

  if (b.kind === 'system') {
    return (
      <div className="py-1 text-center text-[11px] text-muted-foreground/70">
        {b.text}
      </div>
    )
  }

  return (
    <div
      className="py-1"
      style={{ contentVisibility: 'auto', containIntrinsicSize: '0 80px' }}
    >
      <pre className="whitespace-pre-wrap break-words font-mono text-[13px] leading-[1.5] text-foreground/90">
        {b.text}
      </pre>
    </div>
  )
})

/**
 * Lite shell reader. Performance: content-visibility rows, throttled parent
 * updates, scroll only when pinned to bottom — no live region spam.
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
  const itemCount = items.length
  const last = items[items.length - 1]
  const lastId = last?.id
  const lastLen = !last
    ? 0
    : last.type === 'block'
      ? last.block.text.length
      : last.text.length

  useEffect(() => {
    const el = listRef.current
    if (!el || !stickBottom.current) return
    // Instant jump — smooth scroll fights rapid stream updates on mobile.
    el.scrollTop = el.scrollHeight
  }, [itemCount, lastId, lastLen])

  useEffect(() => {
    if (!active) return
    const t = window.setTimeout(() => inputRef.current?.focus(), 40)
    return () => window.clearTimeout(t)
  }, [active])

  const onScroll = useCallback(() => {
    const el = listRef.current
    if (!el) return
    stickBottom.current =
      el.scrollHeight - el.scrollTop - el.clientHeight < 120
  }, [])

  const sendDraft = useCallback(() => {
    const text = draft.replace(/\s+$/g, '')
    if (!text || disabled) return
    onSend(text)
    setDraft('')
    stickBottom.current = true
  }, [draft, disabled, onSend])

  const onForm = (e: FormEvent) => {
    e.preventDefault()
    sendDraft()
  }

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      sendDraft()
    }
  }

  const onToggle = useCallback((id: string) => {
    setCollapsed((prev) => ({ ...prev, [id]: !(prev[id] ?? true) }))
  }, [])

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
        className={cn(
          'min-h-0 flex-1 overflow-y-auto overscroll-contain px-3 py-3 sm:px-5 sm:py-4',
          // Isolate scroll work from the rest of the page (mobile Safari).
          '[contain:strict] [transform:translateZ(0)]',
        )}
        role="log"
        aria-live="off"
        aria-label="Terminal stream"
      >
        <div className="mx-auto flex w-full max-w-xl flex-col">
          {empty ? (
            <div className="flex flex-col items-center gap-2 py-16 text-center text-sm text-muted-foreground">
              <p className="max-w-[16rem] leading-relaxed">
                Lite shell reader — prompts, ANSI, echoes and repeats are
                filtered. Type below to send.
              </p>
              {status ? (
                <p className="font-mono text-[11px] text-muted-foreground/80">
                  {status}
                </p>
              ) : null}
            </div>
          ) : null}

          {items.map((item) => (
            <FeedRow
              key={item.id}
              item={item}
              baseId={baseId}
              collapsed={collapsed}
              onToggle={onToggle}
            />
          ))}
        </div>
      </div>

      <form
        onSubmit={onForm}
        className={cn(
          'shrink-0 border-t border-border bg-card/95 px-3 py-2',
          'backdrop-blur supports-[backdrop-filter]:bg-card/85',
          'pb-[max(0.5rem,env(safe-area-inset-bottom))]',
        )}
      >
        <div className="mx-auto flex w-full max-w-xl items-end gap-2">
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
            placeholder="Message the shell…"
            className={cn(
              'max-h-28 min-h-11 flex-1 resize-none rounded-2xl border border-input',
              'bg-background px-3.5 py-2.5 text-base leading-snug text-foreground',
              'placeholder:text-muted-foreground',
              'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
              'disabled:opacity-50 sm:min-h-10 sm:text-sm',
            )}
          />
          <Button
            type="submit"
            size="icon"
            disabled={disabled || !draft.trim()}
            className="size-11 shrink-0 touch-manipulation rounded-full sm:size-10"
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
