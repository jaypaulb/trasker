# Server (internal/server)

## Purpose
The self-hosted Go server: HTTP API (Chi router), Postgres persistence, dual auth (Entra OIDC for
humans, bcrypt API keys for client daemons), and on-demand delivery of pre-compiled client binaries.
Serves the embedded SvelteKit dashboard SPA. Entry point is `cmd/trasker-server/main.go`.

## Packages
- `api/` — HTTP handlers + `router.go`. Handlers grouped by resource: `auth`, `local_auth`, `user`,
  `admin`, `device`, `timesheet`, `report`, `layout`, `build`, `health`. `respond.go` is the JSON
  response helper; `store_adapter.go` bridges handlers to the store interface.
- `store/` — Postgres data access via `pgx`. One file per aggregate: `users`, `apikeys`, `devices`,
  `timesheets`, `reports`, `settings` (`org_settings`), `audit`, `layout_snapshots`. `store.go`
  defines the interface the API depends on.
- `auth/` — `oidc.go` (Entra ID), `jwt.go` (dashboard session tokens), `apikey.go` (client key
  verification), `roles.go` (admin/manager/member RBAC), `context.go` (request-scoped identity).
- `builder/` — patches sentinel-stamped client binaries per download (see deploy/AGENTS.md).
- `secrets/` — JWT secret resolution (env → `/data/jwt-secret` file → generate).
- `webui/` — `embed.go` (`//go:embed all:static`) serving the server-ui SPA build.

## Contracts & Invariants
- **RBAC is three roles:** admin > manager > member. Enforce via `auth/roles.go` middleware, never
  ad-hoc role checks in handlers. Managers see their team; members see only themselves.
- **API keys are bcrypt-hashed at rest**, identified by `key_prefix`. Never store or log the raw key.
  Keys expire after `org_settings.key_expiry_days` (default 60) of inactivity; admin-revocable.
  One key may register multiple devices via `(client_device_id, api_key_id)` upsert.
- **Timesheet mutation is audit-logged and admin-only.** Every correction writes an `audit` row.
  Clients submit immutable entries; they never edit.
- **All timestamps are `TIMESTAMPTZ`** and travel as UTC. Do not store naive local times.
- **Wire types come from `internal/shared/models`** — do not define parallel request/response structs.
- **Handlers stay thin:** validate → call store → `respond`. Business rules live in the store or a
  dedicated package, not in HTTP handlers.

## Patterns
Add an endpoint: write the handler in `api/<resource>_handlers.go`, register it in `api/router.go`
under the correct auth+role middleware, add the store method to the relevant `store/*.go` (and the
`store.go` interface), and reuse/extend a `models` DTO. Each handler and store method has a `_test.go`.

## Anti-patterns
- Don't bypass `auth/roles.go` with inline role comparisons.
- Don't issue raw SQL from handlers — all DB access goes through `store/`.
- Don't hard-fail on a missing JWT secret; `secrets/jwt.go` owns the env→file→generate precedence.

## Related
- Wire DTOs: `../shared/models` · Schema: `../../migrations/` and `../../deploy/initdb/`
- Deployment-time invariants (autocert, sentinels, admin bootstrap): `../../deploy/AGENTS.md`
- Dashboard SPA source: `../../web/server-ui/` (`web/AGENTS.md`)
