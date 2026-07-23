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
| **API** | `backend/` · `http://localhost:8080` | JWT login, machines CRUD, `POST …/exec`, WS terminal |
| **Web UI** | `web/` · `http://localhost:5173` | Login, machines, xterm.js terminal (ADR-006) |
| **Dummy SSH** | `deploy/ssh-target` · host `2222` | Compose service `ssh-target` (`root`/`targetpass`) |
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

- Prefer **tmux** on the target machine (or an intermediate runner) for session durability.
- Browser disconnect should not destroy the shell; reconnect attaches to the same session when possible.
- Exact remote control channel (SSH + tmux, lightweight agent, etc.) is **TBD** — decide before M3 implementation; record the choice here and any debt in `docs/debt.md`.

---

## 4. Auth flow (conceptual)

1. Prefer Tailscale identity if request presents trusted network identity.
2. Else username/password against local users.
3. Issue JWT for subsequent API and stream auth.
4. Bootstrap admin path for first install and recovery.

---

## 5. Permission enforcement

- Check user ↔ machine and/or user ↔ terminal grants before open/stream/admin actions.
- Default-deny if no grant (aligned with `docs/ddd.md`).
- Emit AuditEvent for security-relevant actions.

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
