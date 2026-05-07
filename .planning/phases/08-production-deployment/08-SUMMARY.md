---
phase: 8
subsystem: deployment
tags: [deployment, tls, autocert, ghcr, ci, docker, embed]
plan_source: docs/superpowers/plans/2026-03-26-production-deployment-plan.md
spec_source: docs/superpowers/specs/2026-03-26-production-deployment-design.md
target_fqdn: trasker.nyolc.cc
target_host: hal
key_decisions:
  - "TLS via golang.org/x/crypto/acme/autocert in-process (no Caddy/nginx)"
  - "Triple-listener (:443 TLS, :80 ACME+redirect, :8080 internal) when TRASKER_FQDN is set; plain :8080 otherwise"
  - "Dashboard SPA embedded via go:embed all:static — no separate static-host container"
  - "Client binaries pre-compiled by CI with sentinel ldflags, copied into image; runtime patches sentinels via bytes.Replace at download time"
  - "Single GHCR image + 2-container compose template (server + postgres) is the canonical production artifact"
metrics:
  tasks_planned: 9
  checkboxes_planned: 42
  checkboxes_completed: 42
  commits_this_phase: 9
  duration_minutes: ~25
---

# Phase 8: Production Deployment Summary

Replaced the legacy 4-container deployment (Caddy + nginx + builder sidecar + Go server + postgres) with a 2-container production stack (Go server + postgres) shipped as a single GHCR image and a one-file compose template. The Go server now absorbs three responsibilities that previously lived in sidecars: TLS termination (autocert), dashboard SPA hosting (`go:embed`), and client-binary distribution (pre-compiled blobs baked in by CI and patched at request time).

## Tasks Completed

| Task | Description | Commit |
|------|-------------|--------|
| 1 | JWT secret auto-generation (env → file → 32-byte random) | `77c100c` |
| 2 | Autocert TLS triple-listener + admin env-var bootstrap | `4790e37` |
| 3 | Builder loads pre-compiled binaries from disk; FQDN-based server URL | `a968583` |
| 4 | Server-side dashboard SPA embed + FQDN-aware OIDC redirect | `859a79d` |
| 5 | Production multi-stage Dockerfile (node + go + alpine) | `6f02da9` |
| 6 | `deploy/docker-compose.production.yml` template | `292a78f` |
| 7 | Removed Caddy, nginx, builder sidecar, legacy compose, .env.example | `6a961bf` |
| 8 | `.github/workflows/release.yml` — clients → image → GHCR → release | `908900d` |
| 9 | Integration smoke test (image build + health + SPA serve) | `b4dd739` |

## Files Created

- `internal/server/secrets/jwt.go` — `LoadOrGenerateJWTSecret(envKey, filePath)`
- `internal/server/secrets/jwt_test.go` — 4 tests covering env/file/generate/precedence
- `internal/server/webui/embed.go` — `go:embed all:static` for dashboard SPA
- `internal/server/webui/static/.gitkeep` — placeholder so `go build` works pre-npm-build
- `deploy/docker-compose.production.yml` — 2-container production template
- `deploy/clients/.gitkeep` — placeholder so Dockerfile `COPY deploy/clients/` works without CI binaries
- `.github/workflows/release.yml` — release pipeline triggered by `v*` tags

## Files Modified

- `cmd/trasker-server/main.go` — autocert triple-listener, JWT loader wiring, admin env-var bootstrap, builder init order (LoadFromDir → PreBuild fallback)
- `internal/server/api/router.go` — `Dependencies.FQDN`, SPA catch-all handler, FQDN-aware `entraRedirectURL()`
- `internal/server/api/build_handlers.go` — prefers `deps.FQDN` over `r.Host` for client `serverURL`
- `internal/server/builder/builder.go` — `NewEmptyBuilder()`, `LoadFromDir()`, `ClientBinDir = "/app/clients"`
- `deploy/Dockerfile.server` — three stages (node:22-slim → golang:1.25-bookworm → alpine:3.20)
- `.gitignore` — added `!.github` / `!.github/**` negation so the workflow file isn't ignored by the blanket `.*` rule
- `go.mod` / `go.sum` — added `golang.org/x/crypto/acme/autocert`; `go mod tidy` promoted previously-indirect direct deps into the explicit require block

## Files Removed

- `deploy/docker-compose.yml` (legacy 4-container production compose)
- `deploy/Dockerfile.builder` + `deploy/builder-entrypoint.sh` (builder sidecar replaced by CI)
- `deploy/caddy/Caddyfile` (autocert replaces it)
- `deploy/nginx/default.conf` (embedded SPA replaces it)
- `deploy/.env.example` (replaced by inline `<CHANGE_ME>` comments in `docker-compose.production.yml`)

## Architectural Notes

**TLS / listener model.** When `TRASKER_FQDN` is set the server runs three concurrent `http.Server` instances:
1. `:443` — TLS via `autocert.Manager` with `HostWhitelist(fqdn)`, cache at `/data/certs`.
2. `:80` — `m.HTTPHandler(nil)`, which serves ACME challenges and redirects everything else to HTTPS.
3. `:8080` — plain HTTP, internal only (not published in compose), used by the container health check and any local-network callers.

When `TRASKER_FQDN` is empty the server falls back to a single plain-HTTP listener on `LISTEN_ADDR` (default `:8080`) — preserves local-dev compatibility.

**JWT secret precedence.** `TRASKER_JWT_SECRET` env > `JWT_SECRET` env (legacy) > `/data/jwt-secret` file > generate 32 random bytes hex-encoded and write to `/data/jwt-secret`. Production compose mounts a named volume at `/data` so the secret persists across container restarts.

**Client binary distribution.** Two paths:
- *Production:* CI pre-compiles client binaries with sentinel ldflags (128-byte placeholders for API key, server URL, version), `COPY deploy/clients/` bakes them into the image, server `LoadFromDir(/app/clients)` verifies sentinels and caches in memory, request-time `Patch()` does `bytes.Replace` to swap real values in.
- *Development:* server detects Go toolchain on PATH, finds module root, calls `PreBuild()` which `go build`s all five targets at startup.

**SPA serving.** Catch-all `r.Get("/*")` at the router tail tries to open the requested path in `webui.Assets/static`; on miss, falls through to `/index.html` for SvelteKit client-side routing.

**CI sentinel safety.** `.github/workflows/release.yml` declares the three sentinels as 128-byte literal env strings and asserts their lengths in a pre-build step; if `internal/server/builder/builder.go` ever changes the sentinel constants, the workflow's assert will fire before any binary is shipped.

**Compose template contract.** Users edit five `<CHANGE_ME>` values (FQDN, admin email/password, postgres password matched in two places). DNS A record + ports 80/443 reachable are documented as preconditions. Optional Entra OIDC block is commented out — uncommenting and filling four values enables SSO.

## Deviations From Plan

| # | Rule | Where | What |
|---|------|-------|------|
| 1 | Rule 3 (blocking) | Task 2 | The plan's Task 2 step 5 wires `FQDN: fqdn` into `Dependencies` before Task 3 step 3 adds the field to the struct. Added the `Dependencies.FQDN` field during Task 2 so main.go would compile; Task 3 step 3's struct edit became a no-op (only the `build_handlers.go` part remained). Documented inline in the plan checkbox. |
| 2 | Rule 3 (blocking) | Task 8 | `.gitignore` had a blanket `.*` rule with `!.git`, `!.gitignore`, `!.planning` negations but **no** negation for `.github`. The new `.github/workflows/release.yml` was being ignored. Added `!.github` / `!.github/**` to the negation list. |
| 3 | Rule 2 (security) | Task 8 | The plan's workflow snippet interpolated `${{ matrix.goos }}` / `${{ matrix.goarch }}` directly inside a `run:` shell block. Refactored to pass them as `env: GOOS:` / `env: GOARCH:` and use `${GOOS}` / `${GOARCH}` shell vars instead — defensive against the GitHub Actions workflow injection class even though matrix values are constrained. |
| 4 | Rule 3 (blocking) | Task 9 | The plan's Task 9 step 2/3 expects the dashboard SPA to be served at `/` from `docker-compose.local.yml`, but `Dockerfile.server-local` builds the **client** SPA (`web/client-ui`), not the dashboard SPA. Smoked the **production** image (`trasker-test:latest`) instead with a postgres-init mount, which is what actually validates the new embedded-SPA path. Documented inline in the plan checkbox. |

## Authentication Gates

None. No auth-required external operations were attempted in this phase (deployment to hal is deferred to a separate operational phase per the objective).

## Threat Flags

None. The phase removes attack surface (proxy sidecars) and adds in-process autocert with a fixed `HostWhitelist` — narrower surface than the previous Caddy config. The `:8080` internal listener is intentionally not published in the production compose `ports:` block, so it is reachable only from inside the container and from other services on the compose network (postgres in this case, which never connects back).

## Smoke Test Result

```
docker build -f deploy/Dockerfile.server -t trasker-test .   # ~5 min, succeeds
docker compose -p trasker-smoke up -d                        # postgres + trasker-test
GET http://localhost:18080/api/v1/health
  -> 200 {"status":"ok","timestamp":"2026-05-07T20:32:07Z","version":"dev (unknown)"}
GET http://localhost:18080/
  -> 200 <SvelteKit dashboard index.html, _app/immutable preloads>
```

Server logs confirmed: `JWT secret loaded from /data/jwt-secret (auto-generated if first boot)`, `bootstrap admin account created`, `no pre-compiled binaries and no Go toolchain — client downloads disabled` (expected — CI populates `/app/clients/` on `v*` tag), `server starting (plain HTTP) addr=:8080`.

## Deferred Issues

- **`internal/client/webui` test setup fails locally.** `go:embed all:static` in `internal/client/webui/embed.go` requires SPA build output that is gitignored. Pre-existing — not caused by Phase 8. Run `cd web/client-ui && npm run build` before `go test ./internal/client/webui/...` if local validation is needed.

## What Phase 9 Will Need (operational handoff)

1. Push a `v*` tag from main → CI builds image, pushes `ghcr.io/jaypaulb/trasker:v*` and `:latest`, attaches `docker-compose.production.yml` to the GitHub Release.
2. On hal: download the compose file, fill in `<CHANGE_ME>` (FQDN=`trasker.nyolc.cc`, admin email/password, postgres password), `docker compose -f docker-compose.production.yml up -d`.
3. DNS: A record `trasker.nyolc.cc` → hal's public IP. Ports 80/443 must be open from the public internet (autocert needs both for the HTTP-01 challenge).
4. First-boot will auto-generate the JWT secret in the `trasker_data` named volume; back this volume up.

## Self-Check: PASSED

All 42 plan checkboxes ticked; all 9 task commits present in `git log 8f32ad8..HEAD`; production Dockerfile builds clean; production image smoke-tested healthy; all server-side Go tests pass.
