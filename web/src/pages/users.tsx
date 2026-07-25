import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { Navigate } from 'react-router'
import { Pencil, Plus, Trash2 } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  createUser,
  deleteUser,
  listUsers,
  updateUser,
  type User,
  type UserRole,
} from '@/lib/api'
import { useAuth } from '@/lib/auth'

export function UsersPage() {
  const { token, user } = useAuth()
  const [users, setUsers] = useState<User[]>([])
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)

  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [role, setRole] = useState<UserRole>('user')

  const [editId, setEditId] = useState<string | null>(null)
  const [editPassword, setEditPassword] = useState('')
  const [editRole, setEditRole] = useState<UserRole>('user')

  const isAdmin = user?.role === 'admin'

  const refresh = useCallback(async () => {
    if (!token || !isAdmin) return
    setLoading(true)
    setError(null)
    try {
      setUsers(await listUsers(token))
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

  async function onCreate(e: FormEvent) {
    e.preventDefault()
    if (!token) return
    setBusy(true)
    setError(null)
    try {
      await createUser(token, { username, password, role })
      setUsername('')
      setPassword('')
      setRole('user')
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  function startEdit(u: User) {
    setEditId(u.id)
    setEditPassword('')
    setEditRole((u.role === 'admin' ? 'admin' : 'user') as UserRole)
    setError(null)
  }

  async function onSaveEdit(e: FormEvent) {
    e.preventDefault()
    if (!token || !editId) return
    setBusy(true)
    setError(null)
    try {
      const body: { password?: string; role?: UserRole } = { role: editRole }
      if (editPassword.trim()) body.password = editPassword
      await updateUser(token, editId, body)
      setEditId(null)
      setEditPassword('')
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  async function onDelete(id: string, name: string) {
    if (!token) return
    if (!window.confirm(`Delete user “${name}”? This cannot be undone.`)) return
    setError(null)
    try {
      await deleteUser(token, id)
      if (editId === id) setEditId(null)
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  return (
    <div className="mx-auto flex max-w-3xl flex-col gap-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Users</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Create and manage local accounts. Only admins can access this page.
          Roles: <code className="text-xs">admin</code> (full access) or{' '}
          <code className="text-xs">user</code> (own machines + explicit grants
          under Permissions).
        </p>
      </div>

      <form
        onSubmit={(e) => void onCreate(e)}
        className="grid gap-3 rounded-lg border border-border bg-card p-4 sm:grid-cols-2"
      >
        <h2 className="sm:col-span-2 text-sm font-semibold">New user</h2>
        <Field label="Username" value={username} onChange={setUsername} />
        <Field
          label="Password"
          value={password}
          onChange={setPassword}
          type="password"
        />
        <label className="flex flex-col gap-1 text-sm">
          <span className="text-muted-foreground">Role</span>
          <select
            className="h-9 rounded-md border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring"
            value={role}
            onChange={(e) => setRole(e.target.value as UserRole)}
          >
            <option value="user">user</option>
            <option value="admin">admin</option>
          </select>
        </label>
        <div className="flex items-end">
          <Button type="submit" disabled={busy || !username || !password}>
            <Plus className="size-4" />
            {busy ? 'Creating…' : 'Create user'}
          </Button>
        </div>
      </form>

      {error ? (
        <p className="text-sm text-destructive" role="alert">
          {error}
        </p>
      ) : null}

      <div className="rounded-lg border border-border">
        <div className="border-b border-border px-4 py-2 text-sm font-medium">
          Local users {loading ? '(loading…)' : `(${users.length})`}
        </div>
        {users.length === 0 && !loading ? (
          <p className="p-4 text-sm text-muted-foreground">No users found.</p>
        ) : (
          <ul className="divide-y divide-border">
            {users.map((u) => (
              <li key={u.id} className="px-4 py-3">
                {editId === u.id ? (
                  <form
                    onSubmit={(e) => void onSaveEdit(e)}
                    className="grid gap-3 sm:grid-cols-2"
                  >
                    <div className="sm:col-span-2">
                      <div className="font-medium">{u.username}</div>
                      <div className="font-mono text-xs text-muted-foreground">
                        {u.id}
                      </div>
                    </div>
                    <Field
                      label="New password (optional)"
                      value={editPassword}
                      onChange={setEditPassword}
                      type="password"
                      required={false}
                    />
                    <label className="flex flex-col gap-1 text-sm">
                      <span className="text-muted-foreground">Role</span>
                      <select
                        className="h-9 rounded-md border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring"
                        value={editRole}
                        onChange={(e) =>
                          setEditRole(e.target.value as UserRole)
                        }
                      >
                        <option value="user">user</option>
                        <option value="admin">admin</option>
                      </select>
                    </label>
                    <div className="sm:col-span-2 flex flex-wrap gap-2">
                      <Button type="submit" size="sm" disabled={busy}>
                        Save
                      </Button>
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        onClick={() => setEditId(null)}
                      >
                        Cancel
                      </Button>
                    </div>
                  </form>
                ) : (
                  <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="font-medium">{u.username}</span>
                        <span className="rounded-md bg-muted px-1.5 py-0.5 text-xs font-medium text-muted-foreground">
                          {u.role}
                        </span>
                        {user?.id === u.id ? (
                          <span className="text-xs text-muted-foreground">
                            (you)
                          </span>
                        ) : null}
                      </div>
                      {u.created_at ? (
                        <div className="text-xs text-muted-foreground">
                          created {new Date(u.created_at).toLocaleString()}
                        </div>
                      ) : null}
                    </div>
                    <div className="flex flex-wrap gap-2">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => startEdit(u)}
                      >
                        <Pencil className="size-4" />
                        Edit
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => void onDelete(u.id, u.username)}
                        aria-label={`Delete ${u.username}`}
                      >
                        <Trash2 className="size-4" />
                      </Button>
                    </div>
                  </div>
                )}
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}

function Field({
  label,
  value,
  onChange,
  type = 'text',
  required = true,
}: {
  label: string
  value: string
  onChange: (v: string) => void
  type?: string
  required?: boolean
}) {
  return (
    <label className="flex flex-col gap-1 text-sm">
      <span className="text-muted-foreground">{label}</span>
      <input
        type={type}
        className="h-9 rounded-md border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        required={required}
        autoComplete={type === 'password' ? 'new-password' : 'username'}
      />
    </label>
  )
}
