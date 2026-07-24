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

**Coolify checklist**
- **web** domain: `https://your.host` → container port **80**
- **api** domain (optional path): `https://your.host/api` → API container port **27341**  
  Coolify **strips** `/api` before the request hits Go. The API therefore accepts both  
  `/api/auth/login` (Compose/nginx) and `/auth/login` (Coolify strip).
- Do **not** set `VITE_API_BASE_URL` to localhost
- Credentials: `BOOTSTRAP_ADMIN_USERNAME` / `BOOTSTRAP_ADMIN_PASSWORD` re-applied every API restart
- After env change: **rebuild/redeploy api + web**
- Prefer leaving **ssh-target** without a public domain (internal only)
- Data in **api** volume; terminal resume via **tmux** on remote host

**Remote channel:** SSH to registered IP/hostname. **Tailscale is not required** for this path (optional mesh later).

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
