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
| Machines | Register / list / delete by IP or hostname + SSH credentials |
| Sessions | 1:N named terminal sessions under a machine; create, switch, close |
| Terminal | Interactive xterm.js UI + one-shot `exec` API for automation |
| Remote channel | SSH (password today; keys later) |
| Ops | Docker Compose: `api`, `web`, optional `ssh-target` for e2e |

**Not in this release (planned):** tmux durability, fine-grained multi-user permissions UI, full audit product, Tailscale identity login.

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
| Web UI | http://localhost:27342 |
| API | http://localhost:27341 |
| Demo SSH host | `localhost:27343` (`root` / `targetpass`) |

**Demo login** (seeded only if missing — set via env for your own instance):

- Username: `admin`
- Password: `123456`

Override with `BOOTSTRAP_ADMIN_USERNAME` / `BOOTSTRAP_ADMIN_PASSWORD` in `.env` before first start.

### First minutes

1. Sign in at the web UI.
2. **Machines** → register the demo host:
   - Address: `ssh-target` (Compose DNS — API talks to the dummy container)
   - Port: `22`
   - User / password: `root` / `targetpass`
3. **Workspace** → select the machine → **New session** (create several for different tasks) → use the terminal pane.

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
| `DATA_DIR` | Persistent store directory (Compose volume `/data`) |
| `VITE_API_BASE_URL` | Browser-facing API origin (default `http://localhost:27341`) |
| `API_PORT` / `WEB_PORT` / `SSH_TARGET_PORT` | Host publish ports (defaults `27341` / `27342` / `27343`) |
| `ACCESS_DEFAULT_TAILSCALE_ONLY` | Network policy default; `false` for open local demos |

---

## Development

```bash
# API
cd backend && go run ./cmd/agent-hub

# Web (proxies /api and /health to :27341)
cd web && npm install && npm run dev

# Tests
cd backend && go test ./...
```

---

## Project layout

```
.
├── backend/           # Go API (auth, machines, terminal sessions, SSH)
├── web/               # React SPA (workspace, machines, login)
├── deploy/ssh-target/ # Minimal OpenSSH container for demos / e2e
├── docs/              # PRD, backlog, ADRs, ops
├── scripts/           # e2e smoke helpers
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

- Bootstrap credentials in `.env.example` are for **local demos only** — change them before any shared or internet-facing deploy.
- SSH passwords are stored in the API data directory for the bridge; treat `DATA_DIR` as sensitive.
- Prefer private networks (VPN / Tailscale / LAN) for production access; TLS and reverse proxies are operator-provided.

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

Actively usable as a **self-hosted skeleton**: login, machines, multi-session workspace, SSH exec/interactive terminal, Compose packaging. Production hardening (keys, tmux persistence, permissions, audit UI) is tracked in the backlog.
