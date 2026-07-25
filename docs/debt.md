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
| D-003 | 2026-07-24 | AuthZ | Terminal ACL inherits machine grants only (no per-terminal grant rows) | Low — enough for v1 sharing; add terminal grants if multi-tenant co-tenancy on one host | open |
| D-006 | 2026-07-24 | Auth | Tailscale identity login (M1.3) not shipped; machine import only | Low — local JWT remains fallback | open |
| D-008 | 2026-07-25 | Access / Tailscale | Reaching a **Tailscale SSH** target depends on the tailnet ACL granting the hub node `"action": "accept"`. The app cannot detect or repair a `check`-mode rule — it can only diagnose it (ADR-007) | Low — the Machines page now flags 100.x:22 rows as Tailscale-SSH targets, says the stored credential is ignored, and offers a preflight check, so the misleading password field no longer sends operators to fix the wrong thing | open |
| D-014 | 2026-07-26 | Terminal / stream view | The readable stream view only parses PTY bytes **while it is the visible mode**; switching to it seeds from xterm's scrollback (last 200 rows) rather than from the true session start. It also drops the tmux status bar by matching our own `ah-<id>` session name, which would stop working if that naming changed | Low — classic xterm is the default and pays nothing for this; the seed keeps the feed useful, and a missed status-bar filter is cosmetic. Next: a small conformance test tying the filter to the store's session-name format | open |
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
| Mobile terminal UX | Soft keys, a sheet picker and a session strip landed in v1.1; the remaining risk is device-specific keyboard behaviour we cannot reproduce in CI | PRD §9, M3.2, M7.3 |
| Exposing beyond mesh | Operators may open WAN for mobile; default-deny and docs must discourage accidental public shells | `docs/ops.md`, M6.* |

---

## Resolved

| ID | Resolved | Note |
|----|----------|------|
| D-004 | 2026-07-26 | **Network policy was recorded intent, not enforcement.** `network_mode` was stored, shown and audited, but nothing ever read it — an operator could read "private_mesh" on the Settings page while the instance answered the whole internet. Resolved by adding a genuinely enforced setting next to it (ADR-013): **Tailnet-only access**, off by default, which refuses any request whose client address is not in Tailscale's ranges. It is deliberately a separate flag rather than a third network mode, because "what I meant" and "what the server refuses" are different claims. Residual: `network_mode` itself is still intent-only, and the app-level check depends on `TRUSTED_PROXIES` being right — the control that actually holds is not publishing the port (docs/ops.md §5d). |
| D-002 | 2026-07-26 | **`npm install` in the web image.** The lockfile had been generated on macOS and was missing `@emnapi/runtime`, a linux-only optional transitive, so `npm ci` refused to run and both the image and CI fell back to `npm install` — which quietly resolves whatever it likes. Resolved by regenerating `web/package-lock.json` inside a linux container (`npm install --package-lock-only`) and switching the Dockerfile and CI to `npm ci`, verified on both linux and macOS. A drifted lockfile now fails the build instead of being papered over. |
| D-005 | 2026-07-26 | **No audit retention control or export.** The cap is now `AUDIT_MAX_EVENTS` (default 1000) and `GET /audit/export` streams every retained event as CSV, with an Export button on the Audit page. Cells starting `=`, `+`, `-` or `@` are prefixed so a crafted username cannot execute when the file is opened in a spreadsheet. Residual: still a ring buffer in one JSON file — export is the retention policy. |
| D-009 | 2026-07-26 | **SSH credentials in plaintext at rest.** Passwords, private keys and key passphrases are now sealed with AES-256-GCM before they reach `store.json` (`enc:v1:` prefix). The key comes from `CREDENTIAL_KEY`, or from `DATA_DIR/credential.key` (32 random bytes, mode 0600) generated on first start. An existing plaintext store is read and re-sealed on the next open. Residual: with the generated key file the key sits in the same directory as the data it protects — set `CREDENTIAL_KEY` from a secret store to separate them. |
| D-011 | 2026-07-26 | **Owner-less machines readable by everyone.** `EnsureBootstrapAdmin` now adopts rows with an empty `owner_user_id` on every start, so a store predating M4 stops being world-readable after one restart. The permissive read path stays in place for a store that has not been through the migration yet. |
| D-013 | 2026-07-26 | **Failed logins pushing history out of the ring.** Partly paid with D-005: the cap is configurable and the log can be exported before it rolls. The underlying shape is unchanged — an unauthenticated caller can still write rows — so edge rate limiting is still the operator's job (see SECURITY.md). |
| D-010 | 2026-07-25 | **`npm audit` high on react-router** (GHSA-qwww-vcr4-c8h2, RSC-mode CSRF bypass). The vulnerable path — RSC / framework mode — was never shipped by this client-only SPA, but the finding was noise for anyone self-hosting. Resolved by moving to `react-router` 8.x and dropping the `react-router-dom` package; the imports were a rename, since every hook and component used here is exported by `react-router` itself. `npm audit` is now clean. |
| D-012 | 2026-07-25 | **No SSH host key verification.** The bridge used `ssh.InsecureIgnoreHostKey()`, accepting any key from anything answering on a machine's address — a man-in-the-middle got the credential and an interactive shell. Resolved by trust-on-first-use pinning (ADR-009): the first successful connect records the fingerprint, later mismatches abort with `host_key_changed`. Residual risk: TOFU still trusts the first connect. |

---

## Template (copy for new rows)

```
| DEBT-NNN | YYYY-MM-DD | area | short description | low/med/high | open |
```
