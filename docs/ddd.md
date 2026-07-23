# Domain Model (DDD sketch) — Agent Hub v1

Language for product and code. Prefer these names in APIs, UI, and schema discussions.

---

## 1. Bounded context

**Terminal Access Control** — identity, machines, terminals, sessions, permissions, and audit for multi-machine web terminals on a Tailscale-centric private network.

No separate “file share” or “remote desktop” contexts in v1.

---

## 2. Core entities

### User

| Attribute | Notes |
|-----------|--------|
| Identity | Local credentials and/or Tailscale identity link |
| Roles | At least operator/admin vs regular user (detail TBD) |
| Auth factors | Username+password fallback; Tailscale preferred when present |

**Invariants:** A user who can act on a resource must be identifiable in the audit log.

### Machine

| Attribute | Notes |
|-----------|--------|
| Display name | Human label |
| IP / address | Entered at manual register |
| Status | Reachable / unknown / disabled (detail TBD) |
| Registration | Manual only (“New device” → IP → register) |

**Invariants:** Machines are not auto-discovered. Registration is an explicit operator action.

### Terminal

| Attribute | Notes |
|-----------|--------|
| Parent | Belongs to exactly one Machine |
| Label | Optional human name |
| Backend session | Backed by a tmux session (or equivalent stable handle) |

**Cardinality:** Machine **1 — N** Terminal.

### Session (terminal session)

| Attribute | Notes |
|-----------|--------|
| Binding | Tied to a Terminal (and thus a Machine) |
| Persistence | tmux-backed; survives browser disconnect when possible |
| Client | xterm.js in the web UI (desktop and mobile browsers) |

**Note:** “Session” here is the durable shell/tmux session, not only the JWT login session.

### Permission

| Attribute | Notes |
|-----------|--------|
| Subject | User |
| Object | Machine and/or Terminal |
| Relation | Many-to-many (user ↔ machine, user ↔ terminal) |
| Effect | Allow (deny/default-deny policy TBD) |

**Invariants:** Access without a matching permission is denied (default-deny assumed unless product later specifies otherwise).

### AuditEvent

| Attribute | Notes |
|-----------|--------|
| Who | User identity |
| When | Timestamp |
| Resource | Machine and/or Terminal |
| Action | Command or control action |
| Outcome | Optional success/failure (recommended) |

**Invariants:** Security-relevant terminal use and admin actions produce audit events. Audit is mandatory for v1.

### AccessPolicy (settings)

| Attribute | Notes |
|-----------|--------|
| Scope | Global (or scoped later) network/access settings |
| Default | Tailscale / private-mesh only ingress |
| Mutable | Changed via Settings UI/API |

---

## 3. Relationships (summary)

```
User ──M:N── Permission ──► Machine
User ──M:N── Permission ──► Terminal
Machine ──1:N── Terminal
Terminal ──1:1?── Session (tmux)   # 1 active durable session per terminal (v1 assumption)
User ──1:N── AuditEvent
Machine / Terminal ◄── referenced by ── AuditEvent
AccessPolicy ── configures ── who may reach the app (network edge)
```

---

## 4. Aggregates (suggested)

| Aggregate root | Contains / owns |
|----------------|-----------------|
| **Machine** | Terminals under that machine; machine registration metadata |
| **User** | Local auth material; permission grants as associations |
| **AuditEvent** | Append-only; no mutation of past events |
| **AccessPolicy** | Singleton or small settings aggregate |

Permission may live as a join entity owned by a dedicated policy service rather than nested deep under User or Machine—implementation choice, not product scope.

---

## 5. Domain rules (v1)

1. Registering a machine requires an explicit IP (or address) input; no auto-discovery.
2. Terminals exist only under a registered machine (1:N).
3. Opening/using a terminal requires permission on that machine and/or terminal.
4. Durable terminal state is provided by tmux, not by the browser alone.
5. AuditEvent is written for identity-bearing terminal commands/actions (and admin mutations).
6. Default network access is Tailscale/private-mesh-only until Settings change AccessPolicy.

---

## 6. Ubiquitous language

| Term | Meaning |
|------|---------|
| Machine | Registered host reachable for terminal control |
| Terminal | One web+tmux terminal instance under a machine |
| Session | Durable tmux-backed shell session for a terminal |
| Permission | Grant linking a user to a machine and/or terminal |
| Audit log | Immutable-ish stream of AuditEvents |
| Access policy | Network/settings gate (default private-mesh/Tailscale-only) |
| Register | Manual machine onboarding by IP |

---

## 7. Out of domain (v1)

- RDP/VNC sessions
- File transfer objects
- Auto-discovered / inventory-only hosts without register
