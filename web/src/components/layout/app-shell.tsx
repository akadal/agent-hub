import { useState } from 'react'
import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { Menu, LogOut } from 'lucide-react'

import { AppSidebar } from '@/components/layout/app-sidebar'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetTitle } from '@/components/ui/sheet'
import { useAuth } from '@/lib/auth'
import { cn } from '@/lib/utils'

export function AppShell() {
  const [mobileOpen, setMobileOpen] = useState(false)
  const { user, loading, logout } = useAuth()
  const location = useLocation()
  const isWorkspace = location.pathname.startsWith('/workspace')

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
      <aside className="hidden w-64 shrink-0 border-r border-sidebar-border md:block">
        <AppSidebar />
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
            className="md:hidden"
            onClick={() => setMobileOpen(true)}
            aria-label="Open navigation"
          >
            <Menu className="size-5" />
          </Button>
          <div className="flex flex-1 flex-col">
            <span className="text-sm font-medium">
              Multi-Machine Terminal Dashboard
            </span>
            <span className="text-xs text-muted-foreground">
              {user.username} · {user.role}
            </span>
          </div>
          <Button variant="ghost" size="sm" onClick={logout}>
            <LogOut className="size-4" />
            Logout
          </Button>
        </header>
        <main
          className={cn(
            'flex-1 overflow-auto',
            isWorkspace ? 'p-2 md:p-3' : 'p-4 md:p-6',
          )}
        >
          <Outlet />
        </main>
      </div>
    </div>
  )
}
