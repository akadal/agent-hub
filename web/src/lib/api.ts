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
  created_at?: string
}

export type UserRole = 'admin' | 'user'

export type Machine = {
  id: string
  name: string
  address: string
  port: number
  ssh_user: string
  /** Whether a key is stored for this machine. The key itself never leaves the API. */
  has_private_key?: boolean
  /**
   * The host's own key, recorded on the first successful connect. Public data —
   * compare it against `ssh-keyscan <address>` to confirm you reached the right
   * machine. Absent until the first connect succeeds.
   */
  host_key_fingerprint?: string
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

/**
 * An error carrying the HTTP status, so callers can tell "this token is
 * rejected" (401/403) apart from "the API blinked" (502, restart, offline).
 * Treating those the same is what silently logged people out mid-session.
 */
export class ApiError extends Error {
  readonly status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

async function apiError(res: Response): Promise<ApiError> {
  return new ApiError(res.status, await parseError(res))
}

/** True when the failure means the credentials are bad, not the network. */
export function isAuthRejection(err: unknown): boolean {
  return err instanceof ApiError && (err.status === 401 || err.status === 403)
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
  if (!res.ok) throw await apiError(res)
  return (await res.json()) as User
}

export type ServerHealth = {
  status: string
  service: string
  /** Release plus VCS revision, e.g. "1.0.0 (84020fd)". Older builds omit it. */
  version?: string
}

/** Unauthenticated — the health probe is what tells you which build answered. */
export async function fetchHealth(): Promise<ServerHealth> {
  const res = await fetch(`${API_BASE}/health`)
  if (!res.ok) throw new Error(await parseError(res))
  return (await res.json()) as ServerHealth
}

/** Rotate your own password. Admin resets go through updateUser instead. */
export async function changeOwnPassword(
  token: string,
  body: { current_password: string; new_password: string },
): Promise<void> {
  const res = await fetch(`${API_BASE}/api/me/password`, {
    method: 'POST',
    headers: authHeaders(token),
    body: JSON.stringify(body),
  })
  if (!res.ok && res.status !== 204) throw new Error(await parseError(res))
}

export async function listUsers(token: string): Promise<User[]> {
  const res = await fetch(`${API_BASE}/api/users`, {
    headers: authHeaders(token),
  })
  if (!res.ok) throw new Error(await parseError(res))
  const data = (await res.json()) as { users: User[] }
  return data.users ?? []
}

export async function createUser(
  token: string,
  body: { username: string; password: string; role: UserRole },
): Promise<User> {
  const res = await fetch(`${API_BASE}/api/users`, {
    method: 'POST',
    headers: authHeaders(token),
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(await parseError(res))
  return (await res.json()) as User
}

export async function updateUser(
  token: string,
  id: string,
  body: { password?: string; role?: UserRole },
): Promise<User> {
  const res = await fetch(`${API_BASE}/api/users/${id}`, {
    method: 'PATCH',
    headers: authHeaders(token),
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(await parseError(res))
  return (await res.json()) as User
}

export async function deleteUser(token: string, id: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/users/${id}`, {
    method: 'DELETE',
    headers: authHeaders(token),
  })
  if (!res.ok && res.status !== 204) throw new Error(await parseError(res))
}

export type MachineGrant = {
  user_id: string
  machine_id: string
  created_at: string
}

export type AuditEvent = {
  id: string
  at: string
  user_id?: string
  username?: string
  action: string
  machine_id?: string
  terminal_id?: string
  detail?: string
}

export type AccessSettings = {
  network_mode: 'private_mesh' | 'open' | string
}

export async function listGrants(token: string): Promise<MachineGrant[]> {
  const res = await fetch(`${API_BASE}/api/grants`, {
    headers: authHeaders(token),
  })
  if (!res.ok) throw new Error(await parseError(res))
  const data = (await res.json()) as { grants: MachineGrant[] }
  return data.grants ?? []
}

export async function createGrant(
  token: string,
  body: { user_id: string; machine_id: string },
): Promise<MachineGrant> {
  const res = await fetch(`${API_BASE}/api/grants`, {
    method: 'POST',
    headers: authHeaders(token),
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(await parseError(res))
  return (await res.json()) as MachineGrant
}

export async function revokeGrant(
  token: string,
  body: { user_id: string; machine_id: string },
): Promise<void> {
  const res = await fetch(`${API_BASE}/api/grants`, {
    method: 'DELETE',
    headers: authHeaders(token),
    body: JSON.stringify(body),
  })
  if (!res.ok && res.status !== 204) throw new Error(await parseError(res))
}

export async function listAudit(token: string): Promise<AuditEvent[]> {
  const res = await fetch(`${API_BASE}/api/audit`, {
    headers: authHeaders(token),
  })
  if (!res.ok) throw new Error(await parseError(res))
  const data = (await res.json()) as { events: AuditEvent[] }
  return data.events ?? []
}

export async function getSettings(token: string): Promise<AccessSettings> {
  const res = await fetch(`${API_BASE}/api/settings`, {
    headers: authHeaders(token),
  })
  if (!res.ok) throw new Error(await parseError(res))
  return (await res.json()) as AccessSettings
}

export async function updateSettings(
  token: string,
  body: { network_mode: string },
): Promise<AccessSettings> {
  const res = await fetch(`${API_BASE}/api/settings`, {
    method: 'PATCH',
    headers: authHeaders(token),
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(await parseError(res))
  return (await res.json()) as AccessSettings
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
    /** Optional PEM key; preferred over the password when present. */
    ssh_private_key?: string
    ssh_key_passphrase?: string
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
    /** Shared PEM key applied to every imported device. Required for a fleet
     *  with `PasswordAuthentication no`, where a password imports nothing usable. */
    ssh_private_key?: string
    ssh_key_passphrase?: string
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

export async function listAllTerminals(
  token: string,
): Promise<TerminalSession[]> {
  const res = await fetch(`${API_BASE}/api/terminals`, {
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

/** Classified outcome of a machine preflight (mirrors sshterm.CheckResult). */
export type MachineCheck = {
  ok: boolean
  elapsed_ms: number
  failure?: {
    kind: string
    message: string
    hint?: string
    approval_url?: string
    retryable: boolean
  }
}

/**
 * Ask the API whether it can reach this machine. Answers the question without
 * opening a terminal, so a broken host is diagnosed from the Machines page
 * instead of by staring at a black xterm.
 */
export async function checkMachine(
  token: string,
  id: string,
): Promise<MachineCheck> {
  const res = await fetch(`${API_BASE}/api/machines/${id}/check`, {
    method: 'POST',
    headers: authHeaders(token),
  })
  if (!res.ok) throw new Error(await parseError(res))
  return (await res.json()) as MachineCheck
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
