# Architecture Notes — Agent Hub v1

Thin technical sketch. Product truth lives in `docs/prd.md`. Domain language lives in `docs/ddd.md`.

---

## 1. System context

```
[Browser: React + xterm.js]
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

---

## 2. Components (planned)

| Component | Responsibility |
|-----------|----------------|
| **Web UI** | React SPA; machine list; terminal panes (xterm.js); settings; permission admin; responsive layout |
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

## 7. Open technical decisions

- DB choice (SQLite vs Postgres, etc.)
- How the control plane reaches tmux on each machine
- Tailscale identity verification details
- WebSocket auth (cookie vs query token vs first-message JWT)
- Mobile terminal UX details (virtual keyboard, font size, pane chrome)
