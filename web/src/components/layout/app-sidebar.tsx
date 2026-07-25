import { useEffect, useState } from 'react'
import { NavLink } from 'react-router'
import {
  ClipboardList,
  LayoutDashboard,
  LayoutPanelLeft,
  Monitor,
  Settings,
  Shield,
  Terminal,
  Users,
} from 'lucide-react'

import { cn } from '@/lib/utils'
import { fetchHealth } from '@/lib/api'
import { ThemeToggle } from '@/components/theme-toggle'
import { useAuth } from '@/lib/auth'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'

type NavItem = {
  to: string
  label: string
  icon: typeof LayoutDashboard
  end?: boolean
  adminOnly?: boolean
}

const navItems: NavItem[] = [
  { to: '/', label: 'Dashboard', icon: LayoutDashboard, end: true },
  { to: '/workspace', label: 'Workspace', icon: LayoutPanelLeft },
  { to: '/machines', label: 'Machines', icon: Monitor },
  { to: '/users', label: 'Users', icon: Users, adminOnly: true },
  { to: '/permissions', label: 'Permissions', icon: Shield, adminOnly: true },
  { to: '/audit', label: 'Audit', icon: ClipboardList, adminOnly: true },
  { to: '/settings', label: 'Settings', icon: Settings },
]

export function AppSidebar({ onNavigate }: { onNavigate?: () => void }) {
  const { user } = useAuth()
  const isAdmin = user?.role === 'admin'
  const visible = navItems.filter((item) => !item.adminOnly || isAdmin)
  const [serverVersion, setServerVersion] = useState<string | null>(null)

  useEffect(() => {
    let alive = true
    fetchHealth()
      .then((h) => {
        if (alive) setServerVersion(h.version ?? null)
      })
      .catch(() => {
        // A version line is decoration; a health blip must not break the nav.
      })
    return () => {
      alive = false
    }
  }, [])

  return (
    <div className="flex h-full flex-col bg-sidebar text-sidebar-foreground">
      <div className="flex h-14 items-center gap-2 px-4">
        <div className="flex size-8 items-center justify-center rounded-md bg-sidebar-primary text-sidebar-primary-foreground">
          <Terminal className="size-4" />
        </div>
        <div className="flex flex-col">
          <span className="text-sm font-semibold leading-none">Agent Hub</span>
          <span className="text-xs text-muted-foreground">open source</span>
        </div>
      </div>
      <Separator className="bg-sidebar-border" />
      <ScrollArea className="flex-1 px-2 py-3">
        <nav className="flex flex-col gap-1">
          {visible.map(({ to, label, icon: Icon, end }) => (
            <NavLink
              key={to}
              to={to}
              end={end}
              onClick={onNavigate}
              className={({ isActive }) =>
                cn(
                  'flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors',
                  isActive
                    ? 'bg-sidebar-accent text-sidebar-accent-foreground'
                    : 'text-sidebar-foreground/80 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground',
                )
              }
            >
              <Icon className="size-4 shrink-0" />
              {label}
            </NavLink>
          ))}
        </nav>
      </ScrollArea>
      <Separator className="bg-sidebar-border" />
      <div className="flex flex-col gap-3 p-4">
        <ThemeToggle className="w-full" />
        <div className="flex flex-col gap-0.5 text-xs text-muted-foreground">
          <span>Machine → sessions · SSH</span>
          {/* Which build is answering — the first thing a bug report needs. */}
          {serverVersion ? <span>v{serverVersion}</span> : null}
        </div>
      </div>
    </div>
  )
}
