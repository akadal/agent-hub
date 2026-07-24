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
      <div className="flex justify-end py-1">
        <div className="max-w-[90%] rounded-2xl bg-neutral-900 px-3.5 py-2 text-[14px] leading-relaxed text-white">
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
      <div className="py-1">
        <button
          type="button"
          className={cn(
            'flex w-full max-w-full items-center gap-1.5 rounded-lg py-1.5 text-left text-[12px]',
            'text-neutral-500 touch-manipulation hover:text-neutral-700',
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
            <span className="min-w-0 truncate font-normal text-neutral-400">
              {b.text.replace(/\s+/g, ' ').slice(0, 64)}
              {b.text.length > 64 ? '…' : ''}
            </span>
          ) : null}
        </button>
        {!isCollapsed ? (
          <pre
            id={panelId}
            className="mt-1 whitespace-pre-wrap break-words border-l-2 border-neutral-200 pl-3 font-mono text-[12px] leading-relaxed text-neutral-500"
          >
            {b.text}
          </pre>
        ) : null}
      </div>
    )
  }

  if (b.kind === 'command') {
    return (
      <div className="py-1">
        <pre className="whitespace-pre-wrap break-words font-mono text-[13px] leading-relaxed text-violet-700">
          {b.text}
        </pre>
      </div>
    )
  }

  if (b.kind === 'system') {
    return (
      <div className="py-1 text-center text-[11px] text-neutral-400">
        {b.text}
      </div>
    )
  }

  return (
    <div className="py-1">
      <pre className="whitespace-pre-wrap break-words font-mono text-[13px] leading-[1.55] text-neutral-800">
        {b.text}
      </pre>
    </div>
  )
})

/**
 * Minimal light stream reader. Renders a capped feed; does not own PTY I/O.
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
  const lastId = items[items.length - 1]?.id

  useEffect(() => {
    const el = listRef.current
    if (!el || !stickBottom.current) return
    el.scrollTop = el.scrollHeight
  }, [itemCount, lastId])

  useEffect(() => {
    if (!active) return
    const t = window.setTimeout(() => inputRef.current?.focus(), 40)
    return () => window.clearTimeout(t)
  }, [active])

  const onScroll = useCallback(() => {
    const el = listRef.current
    if (!el) return
    stickBottom.current =
      el.scrollHeight - el.scrollTop - el.clientHeight < 96
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
        'flex h-full min-h-0 w-full flex-col bg-[#fafafa] text-neutral-900',
        className,
      )}
    >
      <div
        ref={listRef}
        onScroll={onScroll}
        className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-4 py-5 sm:px-6"
        role="log"
        aria-live="off"
        aria-label="Terminal stream"
      >
        <div className="mx-auto flex w-full max-w-xl flex-col">
          {empty ? (
            <div className="flex flex-col items-center gap-2 py-16 text-center text-sm text-neutral-400">
              <p className="max-w-[16rem] leading-relaxed">
                Lite shell reader — prompts, ANSI, echoes and repeats are
                filtered. Type below to send.
              </p>
              {status ? (
                <p className="font-mono text-[11px] text-neutral-400">
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
        className="shrink-0 border-t border-neutral-200/80 bg-[#fafafa] px-3 py-2.5 pb-[max(0.625rem,env(safe-area-inset-bottom))]"
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
              'max-h-28 min-h-11 flex-1 resize-none rounded-2xl border border-neutral-200',
              'bg-white px-3.5 py-2.5 text-base leading-snug text-neutral-900',
              'placeholder:text-neutral-400',
              'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-neutral-300',
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
