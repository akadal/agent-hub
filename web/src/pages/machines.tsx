import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { LayoutPanelLeft, Plus, Trash2 } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  createMachine,
  deleteMachine,
  execOnMachine,
  listMachines,
  type Machine,
} from '@/lib/api'
import { useAuth } from '@/lib/auth'

export function MachinesPage() {
  const { token } = useAuth()
  const [machines, setMachines] = useState<Machine[]>([])
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [name, setName] = useState('ssh-dummy')
  const [address, setAddress] = useState('ssh-target')
  const [port, setPort] = useState(22)
  const [sshUser, setSshUser] = useState('root')
  const [sshPassword, setSshPassword] = useState('targetpass')
  const [busy, setBusy] = useState(false)
  const [execOut, setExecOut] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    if (!token) return
    setLoading(true)
    setError(null)
    try {
      setMachines(await listMachines(token))
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }, [token])

  useEffect(() => {
    void refresh()
  }, [refresh])

  async function onCreate(e: FormEvent) {
    e.preventDefault()
    if (!token) return
    setBusy(true)
    setError(null)
    try {
      await createMachine(token, {
        name,
        address,
        port,
        ssh_user: sshUser,
        ssh_password: sshPassword,
      })
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  async function onDelete(id: string) {
    if (!token) return
    await deleteMachine(token, id)
    await refresh()
  }

  async function onProbe(id: string) {
    if (!token) return
    setExecOut(null)
    try {
      const res = await execOnMachine(token, id, 'echo agent-hub-e2e && whoami')
      setExecOut(
        `exit=${res.exit_code}\nstdout:\n${res.stdout}\nstderr:\n${res.stderr}`,
      )
    } catch (e) {
      setExecOut(e instanceof Error ? e.message : String(e))
    }
  }

  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Machines</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Register hosts by IP/hostname. Open the{' '}
          <Link className="underline" to="/workspace">
            Workspace
          </Link>{' '}
          to run multiple named terminal sessions per machine.
        </p>
      </div>

      <form
        onSubmit={(e) => void onCreate(e)}
        className="grid gap-3 rounded-lg border border-border bg-card p-4 sm:grid-cols-2"
      >
        <h2 className="sm:col-span-2 text-sm font-semibold">New device</h2>
        <Field label="Name" value={name} onChange={setName} />
        <Field
          label="Address (IP / hostname)"
          value={address}
          onChange={setAddress}
        />
        <Field
          label="SSH port"
          value={String(port)}
          onChange={(v) => setPort(Number(v) || 22)}
        />
        <Field label="SSH user" value={sshUser} onChange={setSshUser} />
        <Field
          label="SSH password"
          value={sshPassword}
          onChange={setSshPassword}
          type="password"
        />
        <div className="sm:col-span-2">
          <Button type="submit" disabled={busy}>
            <Plus className="size-4" />
            {busy ? 'Registering…' : 'Register machine'}
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
          Registered machines {loading ? '(loading…)' : `(${machines.length})`}
        </div>
        {machines.length === 0 && !loading ? (
          <p className="p-4 text-sm text-muted-foreground">
            No machines yet. For Compose demo use address{' '}
            <code>ssh-target</code>, user <code>root</code>, password{' '}
            <code>targetpass</code>.
          </p>
        ) : (
          <ul className="divide-y divide-border">
            {machines.map((m) => (
              <li
                key={m.id}
                className="flex flex-col gap-2 px-4 py-3 sm:flex-row sm:items-center sm:justify-between"
              >
                <div>
                  <div className="font-medium">{m.name}</div>
                  <div className="font-mono text-xs text-muted-foreground">
                    {m.ssh_user}@{m.address}:{m.port}
                  </div>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => void onProbe(m.id)}
                  >
                    Probe SSH
                  </Button>
                  <Button variant="default" size="sm" asChild>
                    <Link to={`/workspace/${m.id}`}>
                      <LayoutPanelLeft className="size-4" />
                      Open workspace
                    </Link>
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => void onDelete(m.id)}
                    aria-label="Delete machine"
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>

      {execOut ? (
        <pre className="overflow-auto rounded-md border border-border bg-muted/40 p-3 text-xs">
          {execOut}
        </pre>
      ) : null}
    </div>
  )
}

function Field({
  label,
  value,
  onChange,
  type = 'text',
}: {
  label: string
  value: string
  onChange: (v: string) => void
  type?: string
}) {
  return (
    <label className="flex flex-col gap-1 text-sm">
      <span className="text-muted-foreground">{label}</span>
      <input
        type={type}
        className="h-9 rounded-md border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        required
      />
    </label>
  )
}
