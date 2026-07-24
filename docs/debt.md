# Technical Debt & Watch List — Agent Hub

Track known shortcuts, accepted risks, and items to revisit. Prefer linking a backlog ID when work is scheduled.

**Process**

1. When shipping a shortcut, add a row here (date, area, risk, owner if any).
2. When paying debt, mark status `resolved` and leave a one-line note—do not delete history.
3. Review this file at milestone boundaries (see `docs/backlog.md`).

---

## Active debt

| ID | Date | Area | Description | Risk | Status |
|----|------|------|-------------|------|--------|
| D-001 | 2026-07-23 | FE | v0.1 web shell is shadcn-admin-*aligned* (Vite/React/TS/shadcn layout) not a full vendor copy of satnaing/shadcn-admin demos | Low — patterns match ADR-006; may re-sync primitives as product pages grow | open |
| D-002 | 2026-07-23 | FE build | Web image uses `npm install` (not `npm ci`) due to optional platform lock drift on linux | Medium — less reproducible installs; revisit lockfile generation | open |
| D-003 | 2026-07-24 | AuthZ | Terminal ACL inherits machine grants only (no per-terminal grant rows) | Low — enough for v1 sharing; add terminal grants if multi-tenant co-tenancy on one host | open |
| D-004 | 2026-07-24 | Access | Network policy is stored intent + docs; no in-app IP allowlist | Medium — operators must secure the edge | open |
| D-005 | 2026-07-24 | Audit | File-backed audit capped at 1000 events; no export/retention jobs | Low — fine for small deployments | open |
| D-006 | 2026-07-24 | Auth | Tailscale identity login (M1.3) not shipped; machine import only | Low — local JWT remains fallback | open |
| D-007 | 2026-07-25 | Terminal / mobile | Phone OS can still kill the radio path while locked; we auto-reconnect + server WS/SSH keepalives, but cannot force a suspended mobile NIC to stay up. Edge (Coolify) and remote sshd timeouts remain operator-owned | Medium — UX relies on reconnect + tmux; document host sshd tips on Machines page | open |

---

## Watch list (not debt yet)

Items that are not debt but likely to become painful if ignored during implementation:

| Topic | Why watch | Related |
|-------|-----------|---------|
| Remote execution model | Agent vs SSH/tmux access path is TBD; wrong choice may force rewrite of terminal bridge | `docs/architecture.md`, M3.5 |
| Tailscale identity mechanism | Integration details TBD; dual auth paths can drift | PRD §3, M1.3 |
| Audit volume | Full command logging may grow fast; retention TBD | PRD §7, M5.* |
| Permission inheritance | Machine-level vs terminal-level grants need clear rules to avoid confusing matrix UI | `docs/ddd.md`, M4.* |
| JWT + dual identity | Local user and Tailscale principal mapping must stay consistent in audit “who” | Auth milestones |
| Mobile terminal UX | xterm on small screens / soft keyboards is easy to ship half-broken | PRD §9, M3.2, M7.3 |
| Exposing beyond mesh | Operators may open WAN for mobile; default-deny and docs must discourage accidental public shells | `docs/ops.md`, M6.* |

---

## Resolved

| ID | Resolved | Note |
|----|----------|------|
| — | — | *Empty.* |

---

## Template (copy for new rows)

```
| DEBT-NNN | YYYY-MM-DD | area | short description | low/med/high | open |
```
