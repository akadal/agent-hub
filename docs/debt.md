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
| — | — | — | *None yet — project bootstrap is docs-only.* | — | — |

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
