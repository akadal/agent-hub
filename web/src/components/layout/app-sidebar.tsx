import { NavLink } from 'react-router-dom'
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
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'

const navItems = [
  { to: '/', label: 'Dashboard', icon: LayoutDashboard, end: true },
  { to: '/workspace', label: 'Workspace', icon: LayoutPanelLeft },
  { to: '/machines', label: 'Machines', icon: Monitor },
  { to: '/users', label: 'Users', icon: Users },
  { to: '/permissions', label: 'Permissions', icon: Shield },
  { to: '/audit', label: 'Audit', icon: ClipboardList },
  { to: '/settings', label: 'Settings', icon: Settings },
]

export function AppSidebar({ onNavigate }: { onNavigate?: () => void }) {
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
          {navItems.map(({ to, label, icon: Icon, end }) => (
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
      <div className="p-4 text-xs text-muted-foreground">
        Machine → sessions · SSH
      </div>
    </div>
  )
}
