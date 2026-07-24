/**
 * API origin. Never use a baked-in localhost URL in the browser — that makes
 * production pages call the developer's machine (desktop works "with local data",
 * mobile login fails). Same-origin (empty base) is always correct behind Coolify.
 */
function resolveApiBase(): string {
  const raw = String(import.meta.env.VITE_API_BASE_URL ?? '').trim()
  if (!raw) return ''
  if (/localhost|127\.0\.0\.1/i.test(raw)) return ''
  return raw.replace(/\/$/, '')
}

const API_BASE = resolveApiBase()

export type User = {
  id: string
  username: string
  role: string
}

export type Machine = {
  id: string
  name: string
  address: string
  port: number
  ssh_user: string
  created_at: string
}

export type TerminalSession = {
  id: string
  machine_id: string
  name: string
  status: string
  created_at: string
  updated_at: string
  /** Server-side durable shell name (tmux) when available */
  remote_session?: string
}

export type LoginResult = {
  token: string
  /** null/omitted when JWT has no expiry (forever) */
  expires_at?: string | null
  user: User
}

export type ExecResult = {
  stdout: string
  stderr: string
  exit_code: number
}

function authHeaders(token?: string | null): HeadersInit {
  const h: Record<string, string> = {
    'Content-Type': 'application/json',
  }
  if (token) h.Authorization = `Bearer ${token}`
  return h
}

async function parseError(res: Response): Promise<string> {
  try {
    const j = (await res.json()) as { error?: string }
    return j.error || res.statusText
  } catch {
    return res.statusText
  }
}

export async function login(
  username: string,
  password: string,
): Promise<LoginResult> {
  const res = await fetch(`${API_BASE}/api/auth/login`, {
    method: 'POST',
    headers: authHeaders(),
    body: JSON.stringify({ username, password }),
  })
  if (!res.ok) throw new Error(await parseError(res))
  return (await res.json()) as LoginResult
}

export async function fetchMe(token: string): Promise<User> {
  const res = await fetch(`${API_BASE}/api/me`, {
    headers: authHeaders(token),
  })
  if (!res.ok) throw new Error(await parseError(res))
  return (await res.json()) as User
}

export async function listMachines(token: string): Promise<Machine[]> {
  const res = await fetch(`${API_BASE}/api/machines`, {
    headers: authHeaders(token),
  })
  if (!res.ok) throw new Error(await parseError(res))
  const data = (await res.json()) as { machines: Machine[] }
  return data.machines ?? []
}

export async function createMachine(
  token: string,
  body: {
    name: string
    address: string
    port: number
    ssh_user: string
    ssh_password: string
  },
): Promise<Machine> {
  const res = await fetch(`${API_BASE}/api/machines`, {
    method: 'POST',
    headers: authHeaders(token),
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(await parseError(res))
  return (await res.json()) as Machine
}

export type TailscaleDevice = {
  name: string
  hostname: string
  os: string
  preferred_address: string
  online: boolean
  already_added: boolean
}

export type TailscaleStatus = {
  configured: boolean
  hint?: string
  devices?: TailscaleDevice[]
}

export type TailscaleImportResult = {
  added: Machine[]
  skipped: number
  message: string
}

export async function getTailscaleStatus(
  token: string,
): Promise<TailscaleStatus> {
  const res = await fetch(`${API_BASE}/api/machines/tailscale`, {
    headers: authHeaders(token),
  })
  if (!res.ok) throw new Error(await parseError(res))
  return (await res.json()) as TailscaleStatus
}

export async function importFromTailscale(
  token: string,
  body: {
    ssh_user: string
    ssh_password: string
    port: number
    online_only?: boolean
  },
): Promise<TailscaleImportResult> {
  const res = await fetch(`${API_BASE}/api/machines/tailscale/import`, {
    method: 'POST',
    headers: authHeaders(token),
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(await parseError(res))
  return (await res.json()) as TailscaleImportResult
}

export async function deleteMachine(
  token: string,
  id: string,
): Promise<void> {
  const res = await fetch(`${API_BASE}/api/machines/${id}`, {
    method: 'DELETE',
    headers: authHeaders(token),
  })
  if (!res.ok && res.status !== 204) throw new Error(await parseError(res))
}

export async function listTerminals(
  token: string,
  machineId: string,
): Promise<TerminalSession[]> {
  const res = await fetch(`${API_BASE}/api/machines/${machineId}/terminals`, {
    headers: authHeaders(token),
  })
  if (!res.ok) throw new Error(await parseError(res))
  const data = (await res.json()) as { terminals: TerminalSession[] }
  return data.terminals ?? []
}

export async function createTerminal(
  token: string,
  machineId: string,
  name?: string,
): Promise<TerminalSession> {
  const res = await fetch(`${API_BASE}/api/machines/${machineId}/terminals`, {
    method: 'POST',
    headers: authHeaders(token),
    body: JSON.stringify(name ? { name } : {}),
  })
  if (!res.ok) throw new Error(await parseError(res))
  return (await res.json()) as TerminalSession
}

export async function getTerminal(
  token: string,
  id: string,
): Promise<TerminalSession> {
  const res = await fetch(`${API_BASE}/api/terminals/${id}`, {
    headers: authHeaders(token),
  })
  if (!res.ok) throw new Error(await parseError(res))
  return (await res.json()) as TerminalSession
}

export async function closeTerminal(
  token: string,
  id: string,
): Promise<void> {
  const res = await fetch(`${API_BASE}/api/terminals/${id}`, {
    method: 'DELETE',
    headers: authHeaders(token),
  })
  if (!res.ok && res.status !== 204) throw new Error(await parseError(res))
}

export async function renameTerminal(
  token: string,
  id: string,
  name: string,
): Promise<TerminalSession> {
  const res = await fetch(`${API_BASE}/api/terminals/${id}`, {
    method: 'PATCH',
    headers: authHeaders(token),
    body: JSON.stringify({ name }),
  })
  if (!res.ok) throw new Error(await parseError(res))
  return (await res.json()) as TerminalSession
}

export async function execOnMachine(
  token: string,
  id: string,
  command: string,
): Promise<ExecResult> {
  const res = await fetch(`${API_BASE}/api/machines/${id}/exec`, {
    method: 'POST',
    headers: authHeaders(token),
    body: JSON.stringify({ command }),
  })
  if (!res.ok) throw new Error(await parseError(res))
  return (await res.json()) as ExecResult
}

export async function execOnTerminal(
  token: string,
  id: string,
  command: string,
): Promise<ExecResult> {
  const res = await fetch(`${API_BASE}/api/terminals/${id}/exec`, {
    method: 'POST',
    headers: authHeaders(token),
    body: JSON.stringify({ command }),
  })
  if (!res.ok) throw new Error(await parseError(res))
  return (await res.json()) as ExecResult
}

export function sessionWsUrl(sessionId: string, token: string): string {
  const base = API_BASE || (typeof window !== 'undefined' ? window.location.origin : '')
  const u = new URL(base || 'http://localhost')
  u.protocol = u.protocol === 'https:' ? 'wss:' : 'ws:'
  u.pathname = `/api/terminals/${sessionId}/ws`
  u.search = `token=${encodeURIComponent(token)}`
  return u.toString()
}

/** @deprecated prefer sessionWsUrl */
export function terminalWsUrl(machineId: string, token: string): string {
  const base = API_BASE || (typeof window !== 'undefined' ? window.location.origin : '')
  const u = new URL(base || 'http://localhost')
  u.protocol = u.protocol === 'https:' ? 'wss:' : 'ws:'
  u.pathname = `/api/machines/${machineId}/terminal`
  u.search = `token=${encodeURIComponent(token)}`
  return u.toString()
}

export { API_BASE }
