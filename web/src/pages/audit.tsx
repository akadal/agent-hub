import { useCallback, useEffect, useState } from 'react'
import { Navigate } from 'react-router-dom'

import { Button } from '@/components/ui/button'
import { listAudit, type AuditEvent } from '@/lib/api'
import { useAuth } from '@/lib/auth'

export function AuditPage() {
  const { token, user } = useAuth()
  const isAdmin = user?.role === 'admin'
  const [events, setEvents] = useState<AuditEvent[]>([])
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  const refresh = useCallback(async () => {
    if (!token || !isAdmin) return
    setLoading(true)
    setError(null)
    try {
      setEvents(await listAudit(token))
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }, [token, isAdmin])

  useEffect(() => {
    void refresh()
  }, [refresh])

  if (!isAdmin) {
    return <Navigate to="/" replace />
  }

  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Audit</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Recent security-relevant actions (login, users, machines, terminals,
            grants, settings). Newest first; capped server-side. Credentials are
            never recorded — a password change logs that it changed, not the value.
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={() => void refresh()}>
          Refresh
        </Button>
      </div>

      {error ? (
        <p className="text-sm text-destructive" role="alert">
          {error}
        </p>
      ) : null}

      <div className="overflow-x-auto rounded-lg border border-border">
        <table className="w-full min-w-[40rem] text-left text-sm">
          <thead className="border-b border-border bg-muted/40">
            <tr>
              <th className="px-3 py-2 font-medium">When</th>
              <th className="px-3 py-2 font-medium">Who</th>
              <th className="px-3 py-2 font-medium">Action</th>
              <th className="px-3 py-2 font-medium">Detail</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-border">
            {loading ? (
              <tr>
                <td colSpan={4} className="px-3 py-6 text-muted-foreground">
                  Loading…
                </td>
              </tr>
            ) : events.length === 0 ? (
              <tr>
                <td colSpan={4} className="px-3 py-6 text-muted-foreground">
                  No events yet. Log in or change machines to populate the log.
                </td>
              </tr>
            ) : (
              events.map((e) => (
                <tr key={e.id}>
                  <td className="whitespace-nowrap px-3 py-2 text-xs text-muted-foreground">
                    {e.at ? new Date(e.at).toLocaleString() : '—'}
                  </td>
                  <td className="px-3 py-2 font-medium">
                    {e.username || e.user_id || '—'}
                  </td>
                  <td className="px-3 py-2 font-mono text-xs">{e.action}</td>
                  <td className="max-w-xs truncate px-3 py-2 text-xs text-muted-foreground">
                    {[e.detail, e.machine_id && `m:${e.machine_id.slice(0, 8)}`]
                      .filter(Boolean)
                      .join(' · ') || '—'}
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}
