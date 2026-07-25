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
| D-011 | 2026-07-25 | AuthZ | Machines with an empty `owner_user_id` (rows created before M4 ownership landed) are readable by **any** authenticated user, by design, so upgrades do not lose access | Low — only affects stores predating M4; new rows always carry an owner. Next: a one-off migration assigning legacy rows to the bootstrap admin | open |
| D-006 | 2026-07-24 | Auth | Tailscale identity login (M1.3) not shipped; machine import only | Low — local JWT remains fallback | open |
| D-009 | 2026-07-25 | Secrets | SSH **private keys** now join passwords in the plaintext JSON store (`data/store.json`, mode-limited only by the filesystem). No encryption at rest, no external secret store | Medium — a store file leak now yields keys, not just passwords. Next: encrypt credential fields with a key from env, or delegate to an agent/secret store | open |
| D-008 | 2026-07-25 | Access / Tailscale | Reaching a **Tailscale SSH** target depends on the tailnet ACL granting the hub node `"action": "accept"`. The app cannot detect or repair a `check`-mode rule — it can only diagnose it (ADR-007) | Low — the Machines page now flags 100.x:22 rows as Tailscale-SSH targets, says the stored credential is ignored, and offers a preflight check, so the misleading password field no longer sends operators to fix the wrong thing | open |
| D-010 | 2026-07-25 | FE deps | `react-router` 7.18.1 is inside the range of GHSA-qwww-vcr4-c8h2 (RSC-mode CSRF bypass), so `npm audit` reports a high finding. The advisory covers React Router's **RSC / framework mode**; this app is a client-only SPA (`BrowserRouter`, no server actions, no RSC), so the vulnerable code path is not shipped. No fixed 7.x release exists — the fix landed in `react-router` 8.2.1+, which also drops the `react-router-dom` package | Low — not exploitable in this build, but a noisy `npm audit` for anyone self-hosting. Next: migrate imports from `react-router-dom` to `react-router` v8 | open |
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
| D-012 | 2026-07-25 | **No SSH host key verification.** The bridge used `ssh.InsecureIgnoreHostKey()`, accepting any key from anything answering on a machine's address — a man-in-the-middle got the credential and an interactive shell. Resolved by trust-on-first-use pinning (ADR-009): the first successful connect records the fingerprint, later mismatches abort with `host_key_changed`. Residual risk: TOFU still trusts the first connect. |

---

## Template (copy for new rows)

```
| DEBT-NNN | YYYY-MM-DD | area | short description | low/med/high | open |
```
