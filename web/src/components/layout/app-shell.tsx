import { useCallback, useState } from 'react'
import { Navigate, Outlet, useLocation } from 'react-router'
import { LogOut, Menu, PanelLeftClose, PanelLeftOpen } from 'lucide-react'

import { AppSidebar } from '@/components/layout/app-sidebar'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetTitle } from '@/components/ui/sheet'
import { useAuth } from '@/lib/auth'
import { cn } from '@/lib/utils'

const SIDEBAR_PREF_KEY = 'ah-sidebar-open'

function readDesktopSidebarOpen(): boolean {
  try {
    const v = localStorage.getItem(SIDEBAR_PREF_KEY)
    if (v === '0' || v === 'false') return false
    if (v === '1' || v === 'true') return true
  } catch {
    /* private mode / SSR */
  }
  return true
}

function writeDesktopSidebarOpen(open: boolean) {
  try {
    localStorage.setItem(SIDEBAR_PREF_KEY, open ? '1' : '0')
  } catch {
    /* ignore */
  }
}

/** Notify layout listeners (xterm fit) after sidebar width changes. */
function notifyLayoutResize() {
  requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      window.dispatchEvent(new Event('resize'))
    })
  })
}

export function AppShell() {
  const [mobileOpen, setMobileOpen] = useState(false)
  const [desktopSidebarOpen, setDesktopSidebarOpen] = useState(readDesktopSidebarOpen)
  const { user, loading, logout } = useAuth()
  const location = useLocation()
  const isWorkspace = location.pathname.startsWith('/workspace')

  const setSidebarOpen = useCallback((open: boolean) => {
    setDesktopSidebarOpen(open)
    writeDesktopSidebarOpen(open)
    notifyLayoutResize()
  }, [])

  const onNavToggle = useCallback(() => {
    // Desktop (md+): collapse/expand permanent sidebar for full-width workspace.
    // Mobile: open the sheet drawer (sidebar is not in the document flow).
    if (typeof window !== 'undefined' && window.matchMedia('(min-width: 768px)').matches) {
      setSidebarOpen(!desktopSidebarOpen)
      return
    }
    setMobileOpen(true)
  }, [desktopSidebarOpen, setSidebarOpen])

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center text-sm text-muted-foreground">
        Loading session…
      </div>
    )
  }
  if (!user) {
    return <Navigate to="/login" replace />
  }

  return (
    <div className="flex min-h-screen w-full bg-background">
      <aside
        className={cn(
          'hidden shrink-0 overflow-hidden border-sidebar-border transition-[width,border-color] duration-200 ease-out md:block',
          desktopSidebarOpen
            ? 'w-64 border-r'
            : 'w-0 border-r-0',
        )}
        aria-hidden={!desktopSidebarOpen}
      >
        {/* Fixed inner width so collapse animates cleanly without reflow thrash */}
        <div className="h-full w-64">
          <AppSidebar />
        </div>
      </aside>

      <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
        <SheetContent side="left" className="w-72 p-0">
          <SheetTitle className="sr-only">Navigation</SheetTitle>
          <AppSidebar onNavigate={() => setMobileOpen(false)} />
        </SheetContent>
      </Sheet>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-14 shrink-0 items-center gap-3 border-b border-border px-4 md:px-6">
          <Button
            variant="ghost"
            size="icon"
            onClick={onNavToggle}
            aria-label={
              desktopSidebarOpen ? 'Collapse navigation' : 'Expand navigation'
            }
            title={
              desktopSidebarOpen
                ? 'Hide menu (more room for workspace)'
                : 'Show menu'
            }
          >
            {/* Mobile always uses Menu; desktop shows open/close affordance */}
            <Menu className="size-5 md:hidden" />
            {desktopSidebarOpen ? (
              <PanelLeftClose className="hidden size-5 md:block" />
            ) : (
              <PanelLeftOpen className="hidden size-5 md:block" />
            )}
          </Button>
          <div className="flex min-w-0 flex-1 flex-col">
            <span className="truncate text-sm font-medium">
              Multi-Machine Terminal Dashboard
            </span>
            <span className="truncate text-xs text-muted-foreground">
              {user.username} · {user.role}
            </span>
          </div>
          <Button variant="ghost" size="sm" onClick={logout}>
            <LogOut className="size-4" />
            <span className="hidden sm:inline">Logout</span>
          </Button>
        </header>
        <main
          className={cn(
            'min-h-0 flex-1',
            /* Workspace owns its own scroll/height; page bounce breaks xterm fit. */
            isWorkspace
              ? 'flex overflow-hidden p-0 md:p-3'
              : 'overflow-auto p-4 md:p-6',
          )}
        >
          <Outlet />
        </main>
      </div>
    </div>
  )
}
