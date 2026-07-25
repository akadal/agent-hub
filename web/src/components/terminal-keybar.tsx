import type { MouseEvent, PointerEvent } from 'react'
import { Keyboard } from 'lucide-react'

import { cn } from '@/lib/utils'

/**
 * Soft keys a phone keyboard does not have.
 *
 * Without this row a mobile shell is close to unusable: no Esc (vim, less),
 * no Tab (completion), no Ctrl (^C on a runaway process), no arrows (history).
 * Ordered by how often they are reached for, because the row scrolls.
 */
type KeyDef = {
  label: string
  /** Bytes to send to the PTY. Absent for the sticky Ctrl toggle. */
  data?: string
  title: string
  /** Wider hit area for the keys people jab at in a hurry. */
  wide?: boolean
}

const KEYS: KeyDef[] = [
  { label: 'Esc', data: '\x1b', title: 'Escape', wide: true },
  { label: 'Tab', data: '\t', title: 'Tab — completion', wide: true },
  { label: 'Ctrl', title: 'Ctrl — applies to the next key you type' },
  { label: '^C', data: '\x03', title: 'Ctrl+C — interrupt' },
  { label: '↑', data: '\x1b[A', title: 'Up — previous command' },
  { label: '↓', data: '\x1b[B', title: 'Down' },
  { label: '←', data: '\x1b[D', title: 'Left' },
  { label: '→', data: '\x1b[C', title: 'Right' },
  { label: '|', data: '|', title: 'Pipe' },
  { label: '~', data: '~', title: 'Home directory' },
  { label: '/', data: '/', title: 'Slash' },
  { label: '-', data: '-', title: 'Dash' },
  { label: '_', data: '_', title: 'Underscore' },
  { label: '^D', data: '\x04', title: 'Ctrl+D — end of input' },
  { label: '^Z', data: '\x1a', title: 'Ctrl+Z — suspend' },
  { label: '^L', data: '\x0c', title: 'Ctrl+L — clear' },
  { label: '^R', data: '\x12', title: 'Ctrl+R — search history' },
]

/**
 * Press handlers for a key that must not steal focus from the terminal.
 *
 * `preventDefault` on pointerdown is what keeps the soft keyboard up — but on
 * touch it also cancels the synthetic click that would follow, so the action
 * has to run on pointerdown itself. `onClick` is kept for keyboard activation
 * only, which reports `detail === 0`; without that guard every tap would fire
 * twice on a mouse.
 */
function pressProps(run: () => void) {
  return {
    onPointerDown: (e: PointerEvent<HTMLButtonElement>) => {
      e.preventDefault()
      run()
    },
    onClick: (e: MouseEvent<HTMLButtonElement>) => {
      if (e.detail === 0) run()
    },
  }
}

export function TerminalKeyBar({
  onSend,
  ctrlArmed,
  onToggleCtrl,
  onToggleKeyboard,
  className,
}: {
  onSend: (data: string) => void
  ctrlArmed: boolean
  onToggleCtrl: () => void
  /** Focus the terminal (raise the soft keyboard) or blur it (dismiss). */
  onToggleKeyboard: () => void
  className?: string
}) {
  return (
    <div
      className={cn(
        'flex shrink-0 items-stretch gap-1 border-t border-border bg-card/95 px-1.5 py-1.5',
        'pb-[max(0.375rem,env(safe-area-inset-bottom))] backdrop-blur',
        'supports-[backdrop-filter]:bg-card/85',
        className,
      )}
      role="toolbar"
      aria-label="Terminal keys"
    >
      <button
        type="button"
        {...pressProps(onToggleKeyboard)}
        title="Show or hide the keyboard"
        aria-label="Show or hide the keyboard"
        className="flex size-9 shrink-0 touch-manipulation items-center justify-center rounded-md border border-border bg-background text-muted-foreground active:bg-muted"
      >
        <Keyboard className="size-4" />
      </button>

      <div className="flex min-w-0 flex-1 gap-1 overflow-x-auto [-ms-overflow-style:none] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
        {KEYS.map((k) => {
          const isCtrl = k.data === undefined
          return (
            <button
              key={k.label}
              type="button"
              {...pressProps(() =>
                isCtrl ? onToggleCtrl() : onSend(k.data!),
              )}
              title={k.title}
              aria-pressed={isCtrl ? ctrlArmed : undefined}
              className={cn(
                'h-9 shrink-0 touch-manipulation rounded-md border px-2.5 font-mono text-[13px] leading-none',
                'flex items-center justify-center transition-colors active:bg-muted',
                k.wide && 'px-3',
                isCtrl && ctrlArmed
                  ? 'border-primary bg-primary text-primary-foreground'
                  : 'border-border bg-background text-foreground',
              )}
            >
              {k.label}
            </button>
          )
        })}
      </div>
    </div>
  )
}
