# Agent Hub

**Open-source multi-machine terminal dashboard.**  
Manage SSH hosts from the browser, open **multiple independent terminal sessions per machine**, and keep each task in its own session — similar in spirit to multi-session agent UIs (Claude Code / Codex-style workspace), without locking you to a cloud vendor.

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![CI](https://github.com/akadal/agent-hub/actions/workflows/ci.yml/badge.svg)](https://github.com/akadal/agent-hub/actions/workflows/ci.yml)
[![Release](https://img.shields.io/badge/release-v1.1.0-brightgreen.svg)](CHANGELOG.md)

Run it **locally with Docker Compose**, or **host your own instance** on a VPS,
Coolify, or anything else that runs containers. Same images either way — see
[Running it](#running-it).

---

## Why Agent Hub?

When you work across several hosts (or several jobs on one host), juggling SSH tabs becomes messy. Agent Hub gives you:

- A **machine inventory** you control (manual register by IP/hostname — no auto-discovery)
- A **session workspace**: under each machine, create named terminals (`build`, `debug`, `logs`, …), switch between them, close what you don’t need
- **Browser terminals** powered by xterm.js over a WebSocket → SSH bridge
- A UI that **works on a phone** — soft keys for Esc / Tab / Ctrl / arrows, because no mobile keyboard has them
- **Self-hosted, single binary + static SPA.** No account, no telemetry, no vendor

Tailscale (or any VPN) can sit under your network path, but **it is not required** — plain SSH by address is enough.

---

## Features

| Area | What’s included |
|------|------------------|
| Auth | Local users, JWT sessions, bootstrap admin from env, self-service password change, failed-login throttle |
| Users & permissions | Multi-user CRUD, admin/user roles, per-machine grants with a management UI |
| Machines | Register / list / delete by IP or hostname; pick-and-choose import from a Tailscale tailnet |
| Credentials | SSH password **or** private key (encrypted keys supported); keys never leave the API and are **encrypted at rest** |
| Host identity | Host keys pinned on first connect; a changed key aborts the session instead of handing over the credential |
| Diagnostics | Per-machine **connection check** that names the cause (unreachable, bad key, auth refused, Tailscale approval pending) instead of failing blind |
| Sessions | 1:N named terminal sessions under a machine; create, switch, close |
| Terminal | Interactive xterm.js UI, **tmux-backed** so sessions survive disconnects, plus a one-shot `exec` API |
| Mobile | Sheet picker, always-visible session strip, and a soft-key bar (Esc / Tab / sticky Ctrl / `^C` / arrows) so a phone keyboard can actually drive a shell |
| Appearance | Light, dark or follow-the-system — terminal palette included |
| Access control | Optional **tailnet-only** mode: refuse every request that did not come from your Tailscale network, login included. Off by default, and it will not let you lock yourself out |
| Reconnect | WebSocket + SSH keepalives and auto-reattach after phone sleep or network flap |
| Audit | Append-only log of logins, machine/user/grant changes and terminal use, with an operator UI and CSV export |
| Ops | Docker Compose (`api`, `web`, optional `ssh-target` for e2e), graceful shutdown, version reported by `/health` and the UI |

**Not in this release (planned):** Tailscale identity login — tailnet-only
*access control* ships, but who you are is still a local account. Per-terminal permission grants; today a grant is per
machine and terminals inherit it. Everything known and deliberately deferred is
listed in [`docs/debt.md`](docs/debt.md).

---

## Running it

Two supported shapes, same images:

| | Local | Your own instance |
|---|---|---|
| For | Trying it out, developing, driving machines from your laptop | Reaching your fleet from anywhere, including your phone |
| Runs on | Docker Desktop / any Docker host | VPS, Coolify, Dokploy, Portainer, plain `docker compose` |
| Access | `http://localhost:27342` | Your domain, TLS at your reverse proxy |
| Setup | [below](#local-docker-compose) | [below](#your-own-instance) |

### Requirements

- Docker + Docker Compose
- (Optional, for local dev without Docker) Go 1.22+, Node 20+

### Local (Docker Compose)

```bash
git clone https://github.com/akadal/agent-hub.git
cd agent-hub
cp .env.example .env   # optional; defaults work for a local demo
docker compose up --build
```

| Service | URL |
|---------|-----|
| Web UI | http://localhost:27342 (container port **80**) |
| API | http://localhost:27341 (also via web `/api` same-origin) |
| Demo SSH host | `localhost:27343` (`root` / `targetpass`) |

**Demo login** (seeded only if missing):

- Username: `admin`
- Password: `123456`

Override with `BOOTSTRAP_ADMIN_USERNAME` / `BOOTSTRAP_ADMIN_PASSWORD` in `.env` before first start, then change it from **Settings → Change password** once you are in. Changing the env value and restarting re-applies it — that is the recovery path if the password is lost; leaving it unchanged never overwrites a password you set in the UI.

#### First minutes

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

> `e2e-smoke.sh` **deletes every machine** in the instance it points at. Run it against a scratch stack only.

### Your own instance

Anything that can run `docker compose` will do — Coolify, Dokploy, a plain VPS.
The five things that actually matter:

1. **Point your domain at the `web` service, container port `80`** (not 27342).
   `web` serves the SPA and proxies `/api` and `/health` to the API on the same
   origin. Leave `VITE_API_BASE_URL` **empty** — that is what keeps the browser
   talking to your domain instead of somebody's localhost.
2. **Do not give `api` a public domain.** It only needs to be reachable from
   `web` on the internal network.
3. **Set real secrets before the first start:** `JWT_SECRET` and
   `BOOTSTRAP_ADMIN_PASSWORD`. The API logs a warning on every boot while they
   are still the published demo values.
4. **Persist and back up `DATA_DIR`** (the `api-data` volume). It holds your
   machines, grants, audit log — and, unless you set `CREDENTIAL_KEY` yourself,
   the key that decrypts the stored SSH credentials. A restored store without
   its key is unreadable, and the API refuses to start rather than connect with
   blank credentials.
5. **Drop the demo target.** `ssh-target` exists for the local demo and e2e;
   there is no reason to run an extra SSH server on your deployment.

```bash
# On the host, after cloning and writing a real .env
docker compose up -d --build
curl -s https://your-host.example/health
# {"service":"agent-hub","status":"ok","version":"1.1.0"}
```

Check that `version` after every deploy — the running build reports itself
there and in the web sidebar, so a stale container is visible immediately.

#### If your SSH targets are on a Tailscale `100.x` address

Containers on a Docker bridge network cannot reach the host's tailnet, so the
SSH handshake times out. Run the API with host networking instead:

```bash
docker compose -f docker-compose.yml -f docker-compose.coolify.yml up -d --build
```

In Coolify you can equivalently set **Network Mode = host** on the `api`
service. Docker Desktop on macOS has no host networking, so there the API runs
outside Docker — see the header of [`docker-compose.yml`](docker-compose.yml)
and [`docs/ops.md`](docs/ops.md) §6b, which also explains why an `"action":
"check"` tailnet ACL can never work for a headless hub.

Upgrades, backup and incident notes: [`docs/ops.md`](docs/ops.md) §3.

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

Copy [`.env.example`](.env.example). Every variable the code reads is listed
there — the app has no hidden knobs. The ones that matter most:

| Variable | Purpose |
|----------|---------|
| `JWT_SECRET` | Signing key for access tokens |
| `JWT_ACCESS_TTL` | `forever` (default) or Go duration (`24h`); no login re-auth when forever |
| `BOOTSTRAP_ADMIN_USERNAME` / `_PASSWORD` | First admin (created if missing) |
| `DATA_DIR` | Persistent store directory (Compose volume `/data`) — holds the store and, by default, the key that opens it |
| `CREDENTIAL_KEY` | Optional. Key that encrypts stored SSH credentials. Unset = generated in `DATA_DIR` on first start |
| `AUDIT_MAX_EVENTS` | Optional. Retained audit rows (default `1000`) |
| `HTTP_ADDR` | API listen address inside the container (default `:27341`) |
| `VITE_API_BASE_URL` | Browser-facing API origin. **Leave empty** for same-origin (`web` proxies `/api`) — the recommended setup behind any reverse proxy |
| `API_PORT` / `WEB_PORT` / `SSH_TARGET_PORT` | Host publish ports (defaults `27341` / `27342` / `27343`) |
| `ACCESS_DEFAULT_TAILSCALE_ONLY` | Network policy default; `false` for open local demos |
| `TRUSTED_PROXIES` | CIDRs of the reverse proxies in front of the API. Needed for tailnet-only mode to identify callers; unset = loopback + private ranges |
| `ACCESS_ENFORCEMENT` | `off` disables the tailnet-only check — the recovery path if you lock yourself out |
| `TAILSCALE_API_KEY` | Optional API access token that enables **Machines → Import from Tailscale** |
| `TAILSCALE_TAILNET` | Tailnet for that key; `-` (default) means the key owner's tailnet |

---

## Development

```bash
# API
cd backend && go run ./cmd/agent-hub

# Web (proxies /api and /health to :27341)
cd web && npm ci && npm run dev

# Tests — the same three suites CI runs
cd backend && gofmt -l . && go vet ./... && go test ./...
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
| [`CHANGELOG.md`](CHANGELOG.md) | What changed, per release |
| [`CONTRIBUTING.md`](CONTRIBUTING.md) | Dev setup, tests, PR expectations |
| [`SECURITY.md`](SECURITY.md) | Reporting a vulnerability; hardening a deploy |
| [`AGENTS.md`](AGENTS.md) | Entry map for humans & coding agents |
| [`docs/prd.md`](docs/prd.md) | Product requirements |
| [`docs/backlog.md`](docs/backlog.md) | Milestones |
| [`docs/architecture.md`](docs/architecture.md) | System sketch |
| [`docs/ops.md`](docs/ops.md) | Run / deploy notes |
| [`docs/decisions.md`](docs/decisions.md) | ADRs |
| [`docs/debt.md`](docs/debt.md) | Known shortcuts and accepted risks |

---

## Security notes

- Bootstrap credentials in `.env.example` are for **local demos only** — change `JWT_SECRET` and `BOOTSTRAP_ADMIN_PASSWORD` before any shared or internet-facing deploy. The API warns at startup while they are unchanged.
- SSH passwords, private keys and key passphrases are **encrypted at rest** (AES-256-GCM) inside the store, so the store file alone is not enough. The API has to open them unattended, so the key is reachable by it: by default `credential.key` in the same `0700` data directory. Set `CREDENTIAL_KEY` to keep it elsewhere.
- Prefer private networks (VPN / Tailscale / LAN) for production access; TLS and reverse proxies are operator-provided. **Settings → Tailnet-only access** enforces this in the app (off by default), but the control that actually holds is not publishing the port: serve the app on the tailnet and there is nothing for the internet to reach. Both are in [`docs/ops.md`](docs/ops.md) §5d. A hostname is not a secret — every name you obtain a certificate for is published in Certificate Transparency logs.
- Reaching a host over **Tailscale SSH** (port 22 on a `100.x` address) authenticates by tailnet ACL, not by the credential you stored. See [`docs/ops.md`](docs/ops.md) §6b.
- Failed logins are throttled **per account** (10 per 5 minutes, then `429`), because behind a reverse proxy every request shares the proxy's address. Broad request-rate limiting belongs at your edge.
- Full policy, including what is *not* a vulnerability report: [`SECURITY.md`](SECURITY.md).

---

## Contributing

Issues and pull requests are welcome — see [`CONTRIBUTING.md`](CONTRIBUTING.md)
for the dev setup, what CI checks, and where design decisions are recorded. For
larger changes, open an issue first so scope stays aligned with
[`docs/prd.md`](docs/prd.md).

Found a security problem? Please report it privately — see
[`SECURITY.md`](SECURITY.md), not a public issue.

Participation is covered by the [Code of Conduct](CODE_OF_CONDUCT.md).

---

## Status

**v1.1.0 — feature-complete and self-hostable.** Login, users and per-machine permissions, machine inventory with connection diagnostics, password or key SSH auth with credentials encrypted at rest, a multi-session tmux-backed workspace that works on a phone, an audit log with export, and Compose packaging all work end to end.

Known gaps are tracked openly in [`docs/debt.md`](docs/debt.md) — the notable ones: Tailscale identity *login* is not implemented (tailnet-only access control is, but authentication is still local JWT), and terminal grants are inherited from the machine rather than set per session.

---

## Credits

Built by **[Emre Akadal](https://github.com/akadal)** — **Claude & Grok
powered**. The code, tests and the documents under [`docs/`](docs/) were written
with those models in the loop; the product decisions, the review and the
shipping are human.

---

## License

MIT — see [LICENSE](LICENSE).
