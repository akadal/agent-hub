# Agent Hub — web

The React SPA: login, machine inventory, the multi-session terminal workspace,
and the admin pages (users, permissions, audit, settings).

Vite + React + TypeScript + Tailwind + shadcn/ui primitives, terminals via
[`@xterm/xterm`](https://github.com/xtermjs/xterm.js) over a WebSocket to the Go
API. See [`../docs/decisions.md`](../docs/decisions.md) ADR-006 for why this
shell, and [`../README.md`](../README.md) to run the whole stack.

## Develop

```bash
npm install
npm run dev      # proxies /api and /health to http://localhost:27341
```

The dev server needs the API running (`cd ../backend && go run ./cmd/agent-hub`).
Point it at a different backend with `VITE_API_PROXY=http://127.0.0.1:27351 npm run dev`
when you want a scratch API instead of your usual one.

```bash
npm run lint     # oxlint
npm test         # node:test over src/lib/*.test.ts
npm run build    # tsc -b && vite build
```

## Layout

```
src/
├── pages/       # one file per route (workspace.tsx is the terminal UI)
├── components/  # layout shell, sidebar, ui/ primitives, chat-style view
└── lib/         # api client, auth context, terminal stream parsing
```

`src/lib/api.ts` is the single place that talks to the backend — add endpoints
there rather than calling `fetch` from a page.

## Build-time config

`VITE_API_BASE_URL` is baked into the bundle. Leave it **empty** for same-origin
(the `web` container's nginx proxies `/api` to the API service) — that is the
right setting behind any reverse proxy. Set it only when the browser must reach
an API on a different origin.
