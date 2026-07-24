# Architecture Notes — Agent Hub v1

Thin technical sketch. Product truth lives in `docs/prd.md`. Domain language lives in `docs/ddd.md`.

---

## 1. System context

```
[Browser: React + shadcn-admin shell + xterm.js (later)]
        │  HTTP(S)  — local or self-hosted
        ▼
[Agent Hub — Go API + WS/stream]
        │  Tailscale / private network
        ▼
[Registered machines — shell via tmux]
```

- **Run modes:** local Docker Compose; self-hosted Compose on any suitable host.
- **Default ingress:** Tailscale / private-mesh only (overridable in Settings).
- **Clients:** desktop and mobile browsers (responsive UI).
- **No** RDP/VNC or file-transfer plane in v1.
- **No** hard dependency on a single hosting vendor.

### Implemented e2e surface

| Piece | Path / port | Notes |
|-------|-------------|--------|
| **API** | `backend/` · `http://localhost:27341` | JWT login, machines CRUD, `POST …/exec`, WS terminal |
| **Web UI** | `web/` · `http://localhost:27342` | Login, machines, xterm.js terminal (ADR-006) |
| **Dummy SSH** | `deploy/ssh-target` · host/container `27343` | Compose service `ssh-target` (`root`/`targetpass`) |
| **Compose** | `docker-compose.yml` | `api` + `web` + `ssh-target` |
| **Remote channel** | ADR-005 | **SSH by IP/hostname** — Tailscale not required |

---

## 2. Components

| Component | Responsibility |
|-----------|----------------|
| **Web UI** | React SPA on **satnaing/shadcn-admin** baseline (Vite, TypeScript, Tailwind, shadcn/ui); machine list; terminal panes (xterm.js later); settings; permission admin; responsive layout |
| **API** | Go HTTP/JSON; auth; CRUD for users, machines, terminals, permissions; audit query |
| **Terminal gateway** | Bidirectional stream (WebSocket or similar) between xterm and remote tmux/PTY |
| **Auth** | JWT; local users; Tailscale identity when available |
| **Store** | Persistent DB for users, machines, terminals, permissions, audit (engine TBD) |
| **Packaging** | Docker Compose for local and deploy |

---

## 3. Terminal persistence

- **Channel:** SSH + PTY (ADR-005). Each Terminal row has a `remote_session` name.
- **Durability:** remote command prefers `tmux -u new-session -A -s <name>` when `tmux` is on PATH (Homebrew paths included); otherwise a plain login shell.
- Browser disconnect closes the WebSocket/SSH client only; **tmux keeps the shell**. Reopen/reattach uses the same `remote_session`.
- **Keepalive stack:** API sends SSH `keepalive@openssh.com` + TCP keepalive; WebSocket protocol Pings from the server (~25s) so reverse proxies and idle paths do not depend on mobile JS timers; browser auto-reconnects on close / visibility / online.
- Explicit Close in the UI kills the remote tmux session when present.

---

## 4. Auth flow (conceptual)

1. Prefer Tailscale identity if request presents trusted network identity.
2. Else username/password against local users.
3. Issue JWT for subsequent API and stream auth.
4. Bootstrap admin path for first install and recovery.

---

## 5. Permission enforcement

- **Access:** admin (all) · machine owner · explicit `MachineGrant` · legacy machines with empty owner.
- **Terminals inherit machine access** (no separate terminal grant table in MVP).
- **Manage (delete machine):** owner or admin only.
- Default-deny for non-owners without a grant.
- Audit events on login, machine/terminal lifecycle, exec, grants, settings.

## 5b. Access policy (settings)

- Stored `network_mode`: `private_mesh` (default) or `open`.
- Records operator intent; actual network edge is reverse-proxy / Tailscale / firewall.

---

## 6. Deploy topology

| Piece | Intent |
|-------|--------|
| Docker Compose | App + dependencies; works the same locally and on a remote host |
| Reverse proxy / TLS | Operator-provided when exposing beyond localhost |
| CI/CD | Optional operator wiring; not required by the product |

---

## 7. Related ADRs

- ADR-001 — product stack (Go, React, tmux, JWT, Compose)
- ADR-006 — FE shell: satnaing/shadcn-admin (Vite + React + TS + shadcn/ui)

## 8. Open technical decisions

- DB choice (file store today; SQLite/Postgres later)
- tmux session persistence on top of SSH (M3.3)
- SSH key auth / secret vault (password OK for local e2e)
- Tailscale identity verification details (optional)
- Mobile terminal UX polish
