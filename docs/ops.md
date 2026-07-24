# Ops & Runbook — Agent Hub

Operational notes for local run, self-hosted deploy, and day-2. Product scope: `docs/prd.md`. Architecture: `docs/architecture.md`.

**Public-repo rule:** Use placeholders (`your-host.example`, env vars). Do not commit private hostnames, personal domains, or real secrets.

---

## 1. Environments

| Mode | Intent |
|------|--------|
| **Local** | Primary dev and single-operator use via Docker Compose |
| **Self-hosted deploy** | Same Compose stack on a VPS, home server, or any container host—required if you want phone/off-LAN access |
| **CI deploy** | Optional; operators wire their own pipeline if they want commit → ship |

| Item | Default / note |
|------|----------------|
| Packaging | Docker Compose |
| Network default | Tailscale / private-mesh only (configurable in app Settings) |
| TLS / reverse proxy | Operator-provided when exposing beyond localhost |

No specific PaaS or control panel is required. Coolify, Traefik, Caddy, nginx, cloud load balancers, etc. are examples operators may choose.

---

## 2. Local run (working e2e)

1. Clone the repository.
2. Optional: `cp .env.example .env` — bootstrap admin defaults to **admin / 123456** (change in real deploys).
3. From repo root: `docker compose up --build`
4. Open **http://localhost:27342** → sign in with bootstrap admin.
5. Register a machine:
   - **Address:** `ssh-target` (Compose DNS; API reaches dummy SSH on the same network)
   - **Port:** `27343`
   - **SSH user / password:** `root` / `targetpass`
6. Use **Workspace** → new sessions (xterm.js over WebSocket).

| Service | Compose name | Host / container port | Notes |
|---------|--------------|------------------------|--------|
| API | `api` | host **27341** → container **27341** | JWT; machines; SSH |
| Web | `web` | host **27342** → container **80** | SPA; Coolify must target **80** |
| Dummy SSH | `ssh-target` | host **27343** → container **27343** | e2e (`root`/`targetpass`) |

**Coolify checklist (critical)**

| Service | Public domain | Port |
|---------|---------------|------|
| **web** | `https://your.host` **only this one** | **80** |
| **api** | **none** (internal) | 27341 |
| **ssh-target** | **none** (internal) | 27343 |

If **both** web and api have the same domain (`agents.example`), Coolify may send **all** traffic to **api**. Then `/` is a Go 404 (no SPA), while `/api/hello` still works. Fix: **remove domain from api**, keep it only on **web**. Web nginx proxies `/api` → `api:27341` on the Docker network.

- Do **not** set `VITE_API_BASE_URL` to localhost  
- Credentials: `BOOTSTRAP_ADMIN_*` on **api** service, re-applied every restart  
- After env change: restart/redeploy **api**  
- ssh-target: no public domain

**Remote channel:** SSH to registered IP/hostname. **Tailscale is not required** for this path (optional mesh later).

**Optional — Import from Tailscale (Machines UI)**

1. Create an **API access token** (Admin → Settings → Keys), not a node auth key.
2. On the **api** service set:
   - `TAILSCALE_API_KEY=tskey-api-…`
   - `TAILSCALE_TAILNET=-` (default tailnet for the key)
3. Redeploy/restart api. Open **Machines** → **Import from Tailscale** → enter shared SSH user/password/port → add devices.
4. Import uses each device’s Tailscale IPv4 (`100.x`) as address. API host must still be able to **route SSH** to those IPs (typically: api also on the tailnet).
5. Re-import is safe: already-registered addresses are skipped.

Automated smoke:

```bash
API_BASE=http://localhost:27341 FE_BASE=http://localhost:27342 E2E_SSH_PORT=27343 ./scripts/e2e-smoke.sh
```

Local dev without Compose:

```bash
cd backend && go run ./cmd/agent-hub
cd web && npm install && npm run dev   # proxies /api and /health to :27341
# Point a machine at a real SSH host or map ssh-target via host port 27343
```

---

## 3. Deploy (generic)

1. Build or pull images: `docker compose build` (or your registry workflow).
2. Run the same stack on a host reachable from clients that need access (including mobile).
3. Put TLS and hostname in front via your reverse proxy / platform.
4. Restrict ingress (prefer Tailscale or VPN; open WAN only with clear AccessPolicy intent).
5. Verify `GET /health` and the web UI from a second device (e.g. phone).

Optional: hook git push to rebuild/redeploy on your platform of choice. Override ports with `API_PORT` / `WEB_PORT` env if needed.

---

## 4. Bootstrap (first install)

- Create initial admin/local user (mechanism TBD with M0.5 / M1.*).
- Confirm AccessPolicy default remains private-mesh/Tailscale-only unless intentionally opened.
- Register first machine via UI: **New device** → IP → register.
- Grant permissions before non-admin users expect terminal access.

---

## 5. Auth ops

| Mode | When |
|------|------|
| Tailscale identity | Preferred for users on the mesh |
| Username + password | Fallback / non-Tailscale paths |
| Bootstrap admin | First install and recovery |

---

## 6. Mobile access notes

- Mobile browsers need a **deployed or otherwise reachable** instance (not only the laptop’s `localhost` unless the phone shares that network path, e.g. same Tailscale).
- UI must remain usable on small screens (see PRD §9).
- Prefer private mesh access over exposing the control plane to the public internet.

### 6.1 Terminal disconnects on mobile (known cause + mitigations)

**Symptom:** xterm shows disconnect / “session closed” after the phone sleeps, switches apps, or sits idle — even when the target is the same host that runs Agent Hub.

**Why (not JWT):**

| Layer | What happens on mobile |
|-------|------------------------|
| Browser | Safari/Chrome **suspend JS timers** and often **kill idle WebSockets** when the screen locks or the tab is backgrounded. Client-only `setInterval` pings are unreliable. |
| Coolify / Traefik / edge | Idle HTTP/WS connections are closed if no traffic for the edge timeout. |
| Agent Hub API | WebSocket bridge closes → SSH client closes. **tmux on the host keeps the shell.** |
| Target sshd | Some hardened configs drop quiet SSH if client keepalives are ignored/disabled. |

**What we force in-app (no host config required for the common case):**

1. JWT default **`JWT_ACCESS_TTL=forever`** — login does not expire under long sessions.
2. SSH TCP keepalive + `keepalive@openssh.com` every **30s** from the API → host.
3. **Server-side WebSocket Ping** every **25s** (protocol frames; no client JS) + app-level JSON ping as backup.
4. nginx (`web`) `proxy_read/send_timeout 7d`, buffering off for `/api/`.
5. **Client auto-reconnect** with backoff + immediate reconnect on `visibilitychange` / `online` / `pageshow`. Reattach uses the same `remote_session` (tmux).

**Operator checklist when it still drops:**

1. Coolify: public domain only on **web:80**; do not short-timeout the service.
2. Edge idle timeout ≥ a few minutes (WS pings keep it warm while the phone is awake).
3. On the **target machine** (if sshd is strict):

```text
# /etc/ssh/sshd_config (or drop-in under sshd_config.d/)
ClientAliveInterval 30
ClientAliveCountMax 240
# then: sudo systemctl reload sshd
```

4. Install **tmux** on targets so reconnect restores the same shell (otherwise a plain login shell is used and scrollback is not durable).

---

## 7. Incident checklist (draft)

1. Can the site be reached from the expected network (Tailscale / VPN / LAN)?
2. Is JWT login / Tailscale identity failing?
3. Is the target machine still registered and reachable?
4. Is tmux/session bridge healthy on that machine?
5. Check audit log for recent access and errors (when M5 ships).

---

## 8. Related docs

| Need | Doc |
|------|-----|
| What we build | `docs/prd.md` |
| What to implement next | `docs/backlog.md` |
| Domain terms | `docs/ddd.md` |
| Shortcuts / risks | `docs/debt.md` |
| System shape | `docs/architecture.md` |
