import { useCallback, useEffect, useMemo, useState } from 'react'
import { Navigate } from 'react-router'

import { Button } from '@/components/ui/button'
import {
  createGrant,
  listGrants,
  listMachines,
  listUsers,
  revokeGrant,
  type Machine,
  type MachineGrant,
  type User,
} from '@/lib/api'
import { useAuth } from '@/lib/auth'

/**
 * MVP permission matrix: user ↔ machine grants.
 * Terminal access inherits machine access (no per-terminal rows in v1 MVP).
 */
export function PermissionsPage() {
  const { token, user } = useAuth()
  const isAdmin = user?.role === 'admin'

  const [users, setUsers] = useState<User[]>([])
  const [machines, setMachines] = useState<Machine[]>([])
  const [grants, setGrants] = useState<MachineGrant[]>([])
  const [error, setError] = useState<string | null>(null)
  const [busyKey, setBusyKey] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    if (!token || !isAdmin) return
    setError(null)
    try {
      const [u, m, g] = await Promise.all([
        listUsers(token),
        listMachines(token),
        listGrants(token),
      ])
      setUsers(u)
      setMachines(m)
      setGrants(g)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }, [token, isAdmin])

  useEffect(() => {
    void refresh()
  }, [refresh])

  const grantSet = useMemo(() => {
    const s = new Set<string>()
    for (const g of grants) s.add(`${g.user_id}:${g.machine_id}`)
    return s
  }, [grants])

  if (!isAdmin) {
    return <Navigate to="/" replace />
  }

  const regularUsers = users.filter((u) => u.role !== 'admin')

  async function toggle(userId: string, machineId: string, on: boolean) {
    if (!token) return
    const key = `${userId}:${machineId}`
    setBusyKey(key)
    setError(null)
    try {
      if (on) {
        await createGrant(token, { user_id: userId, machine_id: machineId })
      } else {
        await revokeGrant(token, { user_id: userId, machine_id: machineId })
      }
      await refresh()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusyKey(null)
    }
  }

  return (
    <div className="mx-auto flex max-w-5xl flex-col gap-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Permissions</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Grant non-admin users access to machines. Admins always see everything;
          machine owners always access their own hosts. Terminal sessions inherit
          machine access.
        </p>
      </div>

      {error ? (
        <p className="text-sm text-destructive" role="alert">
          {error}
        </p>
      ) : null}

      {machines.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          Register a machine first under Machines.
        </p>
      ) : regularUsers.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          Create a non-admin user under Users, then grant machine access here.
        </p>
      ) : (
        <div className="overflow-x-auto rounded-lg border border-border">
          <table className="w-full min-w-[32rem] text-left text-sm">
            <thead className="border-b border-border bg-muted/40">
              <tr>
                <th className="px-3 py-2 font-medium">User</th>
                {machines.map((m) => (
                  <th key={m.id} className="px-3 py-2 font-medium">
                    <div className="max-w-[8rem] truncate" title={m.name}>
                      {m.name}
                    </div>
                    <div className="truncate font-mono text-[10px] font-normal text-muted-foreground">
                      {m.address}
                    </div>
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {regularUsers.map((u) => (
                <tr key={u.id}>
                  <td className="px-3 py-2">
                    <div className="font-medium">{u.username}</div>
                    <div className="text-xs text-muted-foreground">{u.role}</div>
                  </td>
                  {machines.map((m) => {
                    const key = `${u.id}:${m.id}`
                    const checked = grantSet.has(key)
                    const busy = busyKey === key
                    return (
                      <td key={m.id} className="px-3 py-2">
                        <label className="inline-flex items-center gap-2">
                          <input
                            type="checkbox"
                            className="size-4 rounded border-input"
                            checked={checked}
                            disabled={busy}
                            onChange={(e) =>
                              void toggle(u.id, m.id, e.target.checked)
                            }
                          />
                          <span className="sr-only">
                            {u.username} → {m.name}
                          </span>
                        </label>
                      </td>
                    )
                  })}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <p className="text-xs text-muted-foreground">
        Tip: owners keep access without a checkbox. Use grants only for shared
        machines.
      </p>

      {grants.length > 0 ? (
        <div className="rounded-lg border border-border p-3">
          <div className="mb-2 text-sm font-medium">
            Active grants ({grants.length})
          </div>
          <ul className="flex flex-col gap-1 text-xs text-muted-foreground">
            {grants.map((g) => {
              const uname =
                users.find((u) => u.id === g.user_id)?.username ?? g.user_id
              const mname =
                machines.find((m) => m.id === g.machine_id)?.name ??
                g.machine_id
              return (
                <li
                  key={`${g.user_id}:${g.machine_id}`}
                  className="flex flex-wrap items-center justify-between gap-2"
                >
                  <span>
                    <span className="font-medium text-foreground">{uname}</span>
                    {' → '}
                    <span className="font-medium text-foreground">{mname}</span>
                  </span>
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() =>
                      void toggle(g.user_id, g.machine_id, false)
                    }
                  >
                    Revoke
                  </Button>
                </li>
              )
            })}
          </ul>
        </div>
      ) : null}
    </div>
  )
}
