# Web UIs (web/)

## Purpose
Two Svelte front-ends. Both are built to static SPAs and embedded into a Go binary via `//go:embed` —
neither is deployed as a standalone Node service.

- `client-ui/` — SPA served by the **client daemon** on localhost. Routes: timeline, submit, tags,
  pomodoro, settings. Talks to `internal/client/webui/api.go`.
- `server-ui/` — SvelteKit dashboard SPA served by the **server**. Route groups under `(app)`:
  dashboard, reports (CSV export), team (manager+), devices, layout history, and `admin/`
  (settings, users, corrections, audit). Plus `login`, `auth/callback` (Entra OIDC), `auth/change-password`.

## Contracts & Invariants
- **Built and embedded, not served live.** `make client-ui` builds `client-ui` and copies `build/`
  into `internal/client/webui/static/`. The server SPA build lands in `internal/server/webui/static/`
  (embedded via `embed.go`). A stale `static/` means the UI change never ships — rebuild after edits.
- **SvelteKit runs as a static SPA** (client-side routing). The Go server has a catch-all route that
  falls back to `index.html`; do not rely on SSR or server-side load functions.
- **API types mirror `internal/shared/models`.** `src/lib/types.ts` and `src/lib/api.ts` are the
  hand-maintained TS view of the Go wire contract — update them together when a DTO changes.
- **Timestamps arrive as UTC; convert to local for display only.** Submission payloads stay UTC.
- **Auth:** server-ui uses Entra OIDC (`src/lib/auth.ts`, `stores/auth.ts`, `auth/callback`);
  client-ui is unauthenticated localhost-only. Role gating (admin/manager/member) mirrors the server's
  RBAC — a hidden UI control is not a security boundary; the server still enforces.

## Patterns
Add a dashboard view: create the route under `server-ui/src/routes/(app)/`, add API calls in
`src/lib/api.ts` with types in `src/lib/types.ts`, reuse components from `src/lib/components/`
(`BarChart`, `TagChart`, `SummaryCard`, `DateRangeFilter`, `LayoutSnapshot*`, etc.), gate by role via
the auth store, then rebuild+embed.

## Anti-patterns
- Don't add a runtime backend dependency or expect a Node server in production.
- Don't invent response shapes that don't exist in `internal/shared/models`.
- Don't treat client-side role checks as authorization.

## Related
- Client API surface: `../internal/client/webui/api.go` · Server API: `../internal/server/api/`
- Wire contract: `../internal/shared/models`
