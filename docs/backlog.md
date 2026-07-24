# Backlog — Agent Hub v1

Ordered milestones for the Multi-Machine Terminal Dashboard. Items are product/engineering epics, not sprint tickets.

**Status key:** `todo` · `in_progress` · `done` · `blocked`

---

## Milestone 0 — Repo & platform skeleton

| ID | Item | Status |
|----|------|--------|
| M0.1 | Go backend module + health endpoint | done |
| M0.2 | React frontend shell (responsive layout baseline; satnaing/shadcn-admin) | done |
| M0.3 | Docker Compose for local run (app + deps) | done |
| M0.4 | Deploy docs: self-host the same Compose stack (generic; no required PaaS) | done |
| M0.5 | Local JWT auth + user store (bootstrap admin) | done |

---

## Milestone 1 — Authentication

| ID | Item | Status |
|----|------|--------|
| M1.1 | Username + password login (fallback path) | done |
| M1.2 | JWT issue/refresh/logout | done |
| M1.3 | Tailscale identity integration (preferred when available) | todo |
| M1.4 | Bootstrap / recovery admin path for self-hosted ops | done |

---

## Milestone 2 — Machines

| ID | Item | Status |
|----|------|--------|
| M2.1 | Manual register: “New device” → IP → persist machine | done |
| M2.2 | List / view / disable or remove machines | done |
| M2.3 | Reachability check (best-effort; no auto-discovery) | done |

---

## Milestone 3 — Terminals & sessions

| ID | Item | Status |
|----|------|--------|
| M3.1 | 1:N terminals under a machine (named sessions + workspace UI) | done |
| M3.2 | xterm.js UI for interactive shell (desktop + usable mobile) | done |
| M3.3 | tmux-backed persistent sessions | done |
| M3.4 | Reconnect / reattach after browser disconnect | done |
| M3.5 | Backend bridge: dashboard ↔ remote shell via **SSH** (ADR-005) | done |

---

## Milestone 4 — Users & permissions

| ID | Item | Status |
|----|------|--------|
| M4.1 | Multi-user CRUD (or invite/bootstrap) | done |
| M4.2 | Many-to-many grants: user ↔ machine | done |
| M4.3 | Many-to-many grants: user ↔ terminal | done |
| M4.4 | Enforce matrix on open/use/admin APIs | done |
| M4.5 | Permission management UI | done |

> **MVP note (M4.3):** terminal access **inherits** machine grants (no separate terminal ACL rows). Sufficient for v1 multi-user; finer terminal ACL later if needed.

---

## Milestone 5 — Audit log

| ID | Item | Status |
|----|------|--------|
| M5.1 | AuditEvent model: who, when, machine/terminal, command/action | done |
| M5.2 | Write path on terminal use and admin mutations | done |
| M5.3 | Query / export UI for operators | done |

> **MVP note:** append-only JSON store (cap 1000); admin list UI; no CSV export yet.

---

## Milestone 6 — Access settings

| ID | Item | Status |
|----|------|--------|
| M6.1 | Settings: network access policy | done |
| M6.2 | Default: Tailscale / private-mesh only | done |
| M6.3 | Persist and apply policy changes | done |

> **MVP note:** policy is **intent + audit**; edge enforcement remains reverse-proxy / mesh (documented).

---

## Milestone 7 — Hardening, mobile UX & v1 ship

| ID | Item | Status |
|----|------|--------|
| M7.1 | End-to-end happy path on private/Tailscale network | done |
| M7.2 | Verify local Compose run + one generic remote deploy path | done |
| M7.3 | Responsive / mobile browser smoke (login, machine list, terminal) | done |
| M7.4 | Docs update (README run notes, debt watch list) | done |

> **MVP note:** Compose e2e + scripts/e2e-smoke; mobile fullscreen terminal stabilized; remaining polish is optional.

---

## Explicitly not in v1 backlog

- RDP / VNC
- File transfer
- Automatic device discovery
- Hard dependency on a specific hosting vendor

---

## Icebox / later

- Audit retention jobs
- Finer-grained command policy
- Machine groups / tags
- SSO beyond Tailscale + local
- Official helm chart / non-Compose packaging
