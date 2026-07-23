import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { Activity, LayoutPanelLeft, Server, Shield } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { API_BASE } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { cn } from '@/lib/utils'
import type { ReactNode } from 'react'

type HealthResponse = {
  status: string
  service: string
}

export function DashboardPage() {
  const { user, token } = useAuth()
  const [health, setHealth] = useState<HealthResponse | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    void (async () => {
      try {
        const res = await fetch(`${API_BASE}/health`)
        if (!res.ok) throw new Error(`health HTTP ${res.status}`)
        setHealth((await res.json()) as HealthResponse)
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e))
      }
    })()
  }, [])

  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Dashboard</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Signed in as <strong>{user?.username}</strong> ({user?.role}). Open
          the workspace to manage multiple terminal sessions per machine.
        </p>
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        <Card
          title="API health"
          icon={<Activity className="size-4" />}
          body={
            health ? (
              <p className="font-mono text-sm">status: {health.status}</p>
            ) : (
              <p className="text-sm text-muted-foreground">Loading…</p>
            )
          }
        />
        <Card
          title="Session"
          icon={<Shield className="size-4" />}
          body={
            <p className="break-all font-mono text-xs text-muted-foreground">
              JWT: {token ? `${token.slice(0, 24)}…` : 'none'}
            </p>
          }
        />
      </div>

      {error ? (
        <div
          className={cn(
            'rounded-md border border-destructive/40 bg-destructive/10 px-4 py-3 text-sm text-destructive',
          )}
        >
          {error}
        </div>
      ) : null}

      <div className="flex flex-wrap gap-2">
        <Button asChild>
          <Link to="/workspace">
            <LayoutPanelLeft className="size-4" />
            Open workspace
          </Link>
        </Button>
        <Button variant="outline" asChild>
          <Link to="/machines">
            <Server className="size-4" />
            Manage machines
          </Link>
        </Button>
      </div>
    </div>
  )
}

function Card({
  title,
  icon,
  body,
}: {
  title: string
  icon: ReactNode
  body: ReactNode
}) {
  return (
    <div className="rounded-lg border border-border bg-card p-4 text-card-foreground shadow-sm">
      <div className="mb-3 flex items-center gap-2 text-sm font-medium">
        {icon}
        {title}
      </div>
      {body}
    </div>
  )
}
