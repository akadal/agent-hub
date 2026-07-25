# Agent Hub

**Open-source multi-machine terminal dashboard.**  
Manage SSH hosts from the browser, open **multiple independent terminal sessions per machine**, and keep each task in its own session — similar in spirit to multi-session agent UIs (Claude Code / Codex-style workspace), without locking you to a cloud vendor.

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

---

## Why Agent Hub?

When you work across several hosts (or several jobs on one host), juggling SSH tabs becomes messy. Agent Hub gives you:

- A **machine inventory** you control (manual register by IP/hostname — no auto-discovery)
- A **session workspace**: under each machine, create named terminals (`build`, `debug`, `logs`, …), switch between them, close what you don’t need
- **Browser terminals** powered by xterm.js over a WebSocket → SSH bridge
- **Local-first packaging** with Docker Compose (and a dummy SSH target for demos)

Tailscale (or any VPN) can sit under your network path, but **it is not required** — plain SSH by address is enough.

---

## Features

| Area | What’s included |
|------|------------------|
| Auth | Local users, JWT sessions, bootstrap admin from env |
| Users & permissions | Multi-user CRUD, admin/user roles, per-machine grants with a management UI |
| Machines | Register / list / delete by IP or hostname; import from a Tailscale tailnet |
| Credentials | SSH password **or** private key (encrypted keys supported); keys never leave the API |
| Diagnostics | Per-machine **connection check** that names the cause (unreachable, bad key, auth refused, Tailscale approval pending) instead of failing blind |
| Sessions | 1:N named terminal sessions under a machine; create, switch, close |
| Terminal | Interactive xterm.js UI, **tmux-backed** so sessions survive disconnects, plus a one-shot `exec` API |
| Reconnect | WebSocket + SSH keepalives and auto-reattach after phone sleep or network flap |
| Audit | Append-only log of logins, machine/user/grant changes and terminal use, with an operator UI |
| Ops | Docker Compose: `api`, `web`, optional `ssh-target` for e2e |

**Not in this release (planned):** Tailscale identity login (local JWT is the only auth path today), credential encryption at rest, audit export/retention jobs.

---

## Quick start

### Requirements

- Docker + Docker Compose
- (Optional for local dev) Go 1.22+, Node 20+

### Run with Compose

```bash
git clone https://github.com/akadal/agent-hub.git
cd agent-hub
cp .env.example .env   # optional; defaults work for local demo
docker compose up --build
```

| Service | URL |
|---------|-----|
| Web UI | http://localhost:27342 (container port **80**) |
| API | http://localhost:27341 (also via web `/api` same-origin) |
| Demo SSH host | `localhost:27343` (`root` / `targetpass`) |

**Coolify:** domain → **web** service, exposed port **80** (not 27342).

**Demo login** (seeded only if missing — set via env for your own instance):

- Username: `admin`
- Password: `123456`

Override with `BOOTSTRAP_ADMIN_USERNAME` / `BOOTSTRAP_ADMIN_PASSWORD` in `.env` before first start.

### First minutes

1. Sign in at the web UI.
2. **Machines** → register the demo host:
   - Address: `ssh-target` (Compose DNS — the API talks to the dummy container)
   - Port: `27343`
   - User / password: `root` / `targetpass`
3. Press **Test connection** on the new row. It should report *Reachable — SSH authenticated in … ms*; if not, it names the cause and the fix rather than failing silently.
4. **Workspace** → select the machine → **New session** (create several for different tasks) → use the terminal pane.

Registering a key-authenticated host from the shell instead — the key is read from a file, so no PEM pasting:

```bash
./scripts/register-machine.sh \
  --base http://localhost:27341 \
  --name build-box --address 10.0.0.5 --port 22 --user ops \
  --key ~/.ssh/agent-hub_ed25519
```

Automated smoke (with the stack running):

```bash
./scripts/e2e-smoke.sh
```

---

## Architecture (short)

```
Browser (React + xterm.js)
    │  HTTPS / WS
    ▼
Agent Hub API (Go)  ──JWT──►  local user store
    │
    │  SSH (per session / exec)
    ▼
Your machines (or Compose ssh-target)
```

- UI shell: Vite + React + TypeScript + shadcn/ui ([satnaing/shadcn-admin](https://github.com/satnaing/shadcn-admin) baseline)
- API: Go HTTP + WebSocket, file-backed store for users / machines / sessions
- Remote access: **SSH by IP** ([ADR-005](docs/decisions.md))

---

## Configuration

Copy [`.env.example`](.env.example). Important variables:

| Variable | Purpose |
|----------|---------|
| `JWT_SECRET` | Signing key for access tokens |
| `JWT_ACCESS_TTL` | `forever` (default) or Go duration (`24h`); no login re-auth when forever |
| `BOOTSTRAP_ADMIN_USERNAME` / `_PASSWORD` | First admin (created if missing) |
| `DATA_DIR` | Persistent store directory (Compose volume `/data`) — holds SSH credentials, treat as secret |
| `HTTP_ADDR` | API listen address inside the container (default `:27341`) |
| `VITE_API_BASE_URL` | Browser-facing API origin. **Leave empty** for same-origin (`web` proxies `/api`) — the recommended setup behind any reverse proxy |
| `API_PORT` / `WEB_PORT` / `SSH_TARGET_PORT` | Host publish ports (defaults `27341` / `27342` / `27343`) |
| `ACCESS_DEFAULT_TAILSCALE_ONLY` | Network policy default; `false` for open local demos |
| `TAILSCALE_API_KEY` | Optional API access token that enables **Machines → Import from Tailscale** |
| `TAILSCALE_TAILNET` | Tailnet for that key; `-` (default) means the key owner's tailnet |

The API logs a warning at startup if `JWT_SECRET` or `BOOTSTRAP_ADMIN_PASSWORD` is still the published demo default.

---

## Development

```bash
# API
cd backend && go run ./cmd/agent-hub

# Web (proxies /api and /health to :27341)
cd web && npm install && npm run dev

# Tests — the same three suites CI runs
cd backend && go test ./...
cd web && npm run lint && npm test && npm run build
```

Diagnosing an SSH target the way the API does, without going through the UI:

```bash
cd backend && AGENT_HUB_SSH_KEY_FILE=~/.ssh/id_ed25519 go run ./cmd/sshdiag 10.0.0.5 22 ops
```

---

## Project layout

```
.
├── backend/           # Go API (auth, machines, terminal sessions, SSH)
├── web/               # React SPA (workspace, machines, login)
├── deploy/ssh-target/ # Minimal OpenSSH container for demos / e2e
├── docs/              # PRD, backlog, ADRs, ops
├── scripts/           # e2e smoke + machine registration helpers
├── .github/workflows/ # CI: Go tests, web build, Compose e2e
├── docker-compose.yml
└── AGENTS.md          # Map for coding agents
```

---

## Documentation

| Doc | Content |
|-----|---------|
| [`AGENTS.md`](AGENTS.md) | Entry map for humans & coding agents |
| [`docs/prd.md`](docs/prd.md) | Product requirements |
| [`docs/backlog.md`](docs/backlog.md) | Milestones |
| [`docs/architecture.md`](docs/architecture.md) | System sketch |
| [`docs/ops.md`](docs/ops.md) | Run / deploy notes |
| [`docs/decisions.md`](docs/decisions.md) | ADRs |

---

## Security notes

- Bootstrap credentials in `.env.example` are for **local demos only** — change `JWT_SECRET` and `BOOTSTRAP_ADMIN_PASSWORD` before any shared or internet-facing deploy. The API warns at startup while they are unchanged.
- SSH passwords **and private keys** are stored unencrypted in the API data directory so the bridge can use them. Treat `DATA_DIR` as secret: it is owner-only (`0700`), but anyone who can read it holds your fleet's credentials.
- Prefer private networks (VPN / Tailscale / LAN) for production access; TLS and reverse proxies are operator-provided. The in-app network policy records intent — it does not block traffic.
- Reaching a host over **Tailscale SSH** (port 22 on a `100.x` address) authenticates by tailnet ACL, not by the credential you stored. See [`docs/ops.md`](docs/ops.md) §6b.

---

## Contributing

Issues and pull requests are welcome. For larger changes, open an issue first and check `docs/prd.md` / `docs/backlog.md` so scope stays aligned.

1. Fork and branch from `main`
2. Keep changes focused; add/adjust tests under `backend/`
3. Run `go test ./...` and (if Compose is up) `./scripts/e2e-smoke.sh`
4. Open a PR with a clear description

---

## License

MIT — see [LICENSE](LICENSE).

---

## Status

**v1 feature-complete and self-hostable.** Login, users and per-machine permissions, machine inventory with connection diagnostics, password or key SSH auth, a multi-session tmux-backed workspace, an audit log, and Compose packaging all work end to end.

Known gaps are tracked openly in [`docs/debt.md`](docs/debt.md) — the notable ones: SSH credentials are stored unencrypted in the data directory, network policy is recorded intent rather than enforcement (secure the edge yourself), and Tailscale identity login is not implemented.
