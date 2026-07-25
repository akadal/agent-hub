import { Monitor, Moon, Sun } from 'lucide-react'

import { useTheme } from '@/lib/theme'
import type { ThemePreference } from '@/lib/theme-pref'
import { cn } from '@/lib/utils'

const OPTIONS: { value: ThemePreference; label: string; icon: typeof Sun }[] = [
  { value: 'light', label: 'Light', icon: Sun },
  { value: 'dark', label: 'Dark', icon: Moon },
  { value: 'system', label: 'System', icon: Monitor },
]

/**
 * Three-way appearance switch. A segmented control rather than a cycling
 * button: with "System" in the set, a single button gives no way to see
 * which of the three you are on without clicking through all of them.
 */
export function ThemeToggle({
  showLabels = false,
  className,
}: {
  showLabels?: boolean
  className?: string
}) {
  const { preference, setPreference } = useTheme()

  return (
    <div
      role="radiogroup"
      aria-label="Appearance"
      className={cn(
        'inline-flex items-center gap-0.5 rounded-lg border border-border bg-muted/40 p-0.5',
        className,
      )}
    >
      {OPTIONS.map(({ value, label, icon: Icon }) => {
        const selected = preference === value
        return (
          <button
            key={value}
            type="button"
            role="radio"
            aria-checked={selected}
            title={`${label} theme`}
            onClick={() => setPreference(value)}
            className={cn(
              'flex min-h-8 flex-1 items-center justify-center gap-1.5 rounded-md px-2 py-1.5 text-xs font-medium',
              'touch-manipulation transition-colors',
              'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
              selected
                ? 'bg-background text-foreground shadow-sm'
                : 'text-muted-foreground hover:text-foreground',
            )}
          >
            <Icon className="size-4 shrink-0" />
            {showLabels ? <span>{label}</span> : null}
            {showLabels ? null : <span className="sr-only">{label}</span>}
          </button>
        )
      })}
    </div>
  )
}
