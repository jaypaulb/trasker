# Production Deployment Design

## Goal

Replace the 4-container deployment (Caddy + Nginx + API + Postgres) with a 2-container setup (API + Postgres) where the Go server handles TLS, SPA serving, and client binary distribution natively. Ship as a single container image on GHCR with a one-file compose template for users.

## Architecture

### Current (local dev)

```
docker-compose.local.yml:
  postgres (16-alpine) ← migrations on first boot
  api (Go, SPA copied into embed dir at Docker build time) ← plain HTTP :8080
```

### Current (production)

```
docker-compose.yml:
  postgres (16-alpine)
  api (Go, no SPA) ← HTTP :8080 internal
  nginx (SPA static files) ← HTTP :80 internal
  caddy (TLS termination) ← HTTPS :443 external
```

### Target (production)

```
docker-compose.production.yml:
  postgres (16-alpine)
  trasker (Go + embedded SPA + autocert TLS) ← HTTPS :443 + HTTP :80 external
```

The Go server is the entire application server:
- Serves the admin/dashboard SPA via `go:embed` (new — must be added to the server; currently only the client binary embeds a SPA)
- Handles TLS via `golang.org/x/crypto/acme/autocert`
- Serves patched client binaries from pre-compiled blobs baked into the image
- Falls back to plain HTTP when `TRASKER_FQDN` is not set (local dev)

### SPA Embedding (new work)

The client binary already embeds its own SPA via `internal/client/webui/embed.go`. The server does **not** currently embed or serve a SPA — in local dev the Dockerfile copies the SPA build output into the client's embed directory, and in production Nginx serves the SPA separately.

New work required:
- Create `internal/server/webui/embed.go` with `//go:embed all:static` (mirrors the client pattern)
- Add a catch-all route to `internal/server/api/router.go` that serves the embedded SPA filesystem, falling back to `index.html` for client-side routing
- The multi-stage Dockerfile copies SPA build output into `internal/server/webui/static/` before `go build` so the embed directive picks up the files

## Autocert TLS Integration

### Behavior

```
if TRASKER_FQDN is set:
  → autocert.Manager with FQDN-only whitelist
  → listen :443 (TLS via autocert, cert cache at /data/certs/)
  → listen :80 (ACME HTTP-01 challenges + HTTP→HTTPS redirect)
  → listen :8080 (internal plain HTTP for health checks + load balancer)
  → three http.Server instances with coordinated graceful shutdown
else:
  → listen on LISTEN_ADDR (default :8080), plain HTTP, single http.Server
  → identical to current local dev behavior
```

### Details

- Uses `golang.org/x/crypto/acme/autocert` (subpackage of existing `x/crypto` dependency)
- `autocert.Manager` handles cert acquisition, renewal, and caching automatically
- Cert cache: `/data/certs/` directory, persisted via `trasker_data` volume
- ACME HTTP-01 challenge requires port 80 reachable from the internet
- Host whitelist contains only the configured FQDN (prevents cert acquisition for arbitrary domains)

### Triple-listener shutdown

When `TRASKER_FQDN` is set, main.go manages three `http.Server` instances (:443 TLS, :80 redirect/ACME, :8080 internal health/LB). On SIGTERM/SIGINT, all three servers are shut down gracefully with coordinated context cancellation. This replaces the current single-server pattern.

### Server URL derivation

When `TRASKER_FQDN` is set, the server derives its public URL as `https://<TRASKER_FQDN>`. This replaces the existing `TRASKER_SERVER_URL` env var (which is removed). The derived URL is used by the builder when patching client binaries with the server address.

When FQDN is not set (local dev), the existing behavior is preserved: URL derived from the request's Host header and scheme.

### No external TLS tooling

No Caddy, no Nginx, no certbot, no cron. The Go binary handles everything.

## CI Pipeline (GitHub Actions)

### Trigger

Push a git tag matching `v*` (e.g., `v1.0.0`).

### Jobs

#### 1. Build client binaries

Cross-compile all 5 targets with `CGO_ENABLED=0` and sentinel placeholders:
- `linux/amd64`, `linux/arm64`
- `darwin/amd64`, `darwin/arm64`
- `windows/amd64`

Output: 5 binaries uploaded as workflow artifacts for the next job.

#### 2. Build + push container image

Multi-stage Dockerfile:

| Stage | Base | Purpose |
|-------|------|---------|
| spa-builder | `node:22-slim` | Build SvelteKit SPA |
| go-builder | `golang:1.25-bookworm` | Build server binary (CGO_ENABLED=0), SPA output copied into `internal/server/webui/static/` before `go build` |
| runtime | `alpine:3.20` | Final image (~50MB) |

Runtime image contents:
- Server binary at `/app/trasker-server`
- SPA static files embedded in the binary via `go:embed`
- Pre-compiled client binaries at `/app/clients/trasker-client-{os}-{arch}`
- Migrations copied directly from the build context (not from a builder stage) at `/app/migrations/`
- Non-root user `trasker:trasker` (uid:1001)
- `ca-certificates` and `tzdata` packages
- `/data/` directory pre-created and owned by `trasker:trasker` (for certs + JWT secret)

Push to:
- `ghcr.io/<owner>/trasker:<tag>` (e.g., `v1.0.0`)
- `ghcr.io/<owner>/trasker:latest`

#### 3. Create GitHub Release

- Attach `docker-compose.production.yml` as a release asset
- Release body: brief getting-started instructions (edit compose, `docker compose up -d`)

### Image tags

Semver tag (`v1.0.0`) plus mutable `latest`. The compose template references `latest` by default.

## Client Binary Distribution

### Build time (CI)

CI cross-compiles all 5 client binaries with sentinel placeholder strings baked in via `-ldflags -X`:
- `SentinelServerURL` (128 bytes, null-padded)
- `SentinelAPIKey` (128 bytes, null-padded)
- `SentinelVersion` (128 bytes, null-padded)

Binaries are copied into the container image at `/app/clients/`.

### Runtime (server)

The builder package loads pre-compiled binaries from `/app/clients/` on startup (disk read, not compilation). `Patch()` still does `bytes.Replace()` on the cached binary blobs at download time (~50ms per request). Same patching logic as today — the only change is the source of the generic binaries.

The server URL patched into client binaries is derived from `TRASKER_FQDN` as `https://<FQDN>` (replacing the previous `TRASKER_SERVER_URL` env var).

If the client binaries directory is empty or missing (e.g., local dev without CI), the builder falls back to runtime compilation if a Go toolchain is available (existing behavior).

## JWT Secret Auto-Generation

### Precedence

1. `TRASKER_JWT_SECRET` env var (if set, use it — escape hatch)
2. `/data/jwt-secret` file (read existing or generate new)

This replaces the current `mustEnvMulti("TRASKER_JWT_SECRET", "JWT_SECRET")` which hard-exits if neither env var is set. The production compose template deliberately omits `TRASKER_JWT_SECRET` to trigger auto-generation.

### First boot flow

1. Check env var — if set, use it, done
2. Check `/data/jwt-secret` — if exists, read it, done
3. Generate 32 random bytes, hex-encode to 64-char string
4. Write to `/data/jwt-secret` (permissions 0600)
5. Use as JWT signing key

### Subsequent boots

Step 2 succeeds — reads existing file. No regeneration.

### Implementation

New package: `internal/server/secrets/jwt.go` — single exported function `LoadOrGenerateJWTSecret(envVar string, filePath string) (string, error)`.

## Admin Bootstrap

**This modifies existing behavior.** Currently `bootstrapAdmin()` in main.go unconditionally creates `admin@localhost` / `trasker-admin` with `force_password_change = true` (defaults from `internal/server/api/local_auth_handlers.go`). The new behavior reads env vars to allow custom credentials.

### On first boot (no users in DB)

| TRASKER_ADMIN_EMAIL | TRASKER_ADMIN_PASSWORD | Result |
|---------------------|------------------------|--------|
| set | set | Create admin with those credentials, `force_password_change = false` |
| unset | unset | Create `admin@localhost` / `trasker-admin`, `force_password_change = true` (existing default behavior) |
| one set, one unset | — | Log warning, skip bootstrap |

### On subsequent boots (users exist)

Skip entirely. The env vars are only consulted once.

## Production Compose Template

```yaml
# Trasker — Production deployment
# 1. Edit the values marked <CHANGE_ME> below
# 2. Run: docker compose -f docker-compose.production.yml up -d

services:
  postgres:
    image: postgres:16-alpine
    restart: unless-stopped
    environment:
      POSTGRES_DB: trasker
      POSTGRES_USER: trasker
      POSTGRES_PASSWORD: <CHANGE_ME>
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U trasker"]
      interval: 3s
      timeout: 2s
      retries: 10

  trasker:
    image: ghcr.io/<owner>/trasker:latest
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
    ports:
      - "443:443"
      - "80:80"
    volumes:
      - trasker_data:/data
    environment:
      TRASKER_FQDN: <CHANGE_ME>
      TRASKER_ADMIN_EMAIL: <CHANGE_ME>
      TRASKER_ADMIN_PASSWORD: <CHANGE_ME>
      TRASKER_DB_HOST: postgres
      TRASKER_DB_NAME: trasker
      TRASKER_DB_USER: trasker
      TRASKER_DB_PASSWORD: <CHANGE_ME>  # must match POSTGRES_PASSWORD
      # --- Optional: Azure AD SSO ---
      # TRASKER_ENTRA_TENANT: <tenant-id>
      # TRASKER_ENTRA_CLIENT: <client-id>
      # TRASKER_ENTRA_SECRET: <client-secret>
      # ENTRA_REDIRECT_URL: https://<your-fqdn>/auth/callback
    healthcheck:
      test: ["CMD", "wget", "--spider", "-q", "http://localhost:8080/api/v1/health"]
      interval: 10s
      timeout: 3s
      retries: 5

volumes:
  pgdata:
  trasker_data:
```

**Note on health check:** In TLS mode, the server still binds an internal HTTP listener on `:8080` (in addition to :443/:80) so the health check can reach `/api/v1/health` without TLS. This also allows placing the container behind an external load balancer if desired.

**Note on ENTRA_REDIRECT_URL:** When OIDC is enabled and `ENTRA_REDIRECT_URL` is not set, the server derives it from `TRASKER_FQDN` as `https://<FQDN>/auth/callback`. Users can override with an explicit value if their setup requires a different callback URL.

User edits 4 values (FQDN, admin email, admin password, postgres password), runs one command.

## Files Changed / Created

### Modified

| File | Change |
|------|--------|
| `cmd/trasker-server/main.go` | Autocert dual-listener logic (FQDN check), JWT secret from file (replaces `mustEnvMulti`), admin bootstrap from env vars, ENTRA_REDIRECT_URL derivation from FQDN |
| `internal/server/api/router.go` | Add catch-all SPA serving route from embedded filesystem, derive ENTRA_REDIRECT_URL from FQDN when not set |
| `internal/server/builder/builder.go` | Load pre-compiled binaries from disk path instead of compiling at startup, derive server URL from FQDN |
| `go.mod` | No new deps — `autocert` is a subpackage of existing `golang.org/x/crypto` |
| `deploy/Dockerfile.server` | Rewrite as production multi-stage (node + go + alpine), copy SPA output into server embed dir, copy client binaries, `RUN mkdir -p /data/certs && chown -R trasker:trasker /data` |

### Created

| File | Purpose |
|------|---------|
| `.github/workflows/release.yml` | CI: build clients, build+push image, create release |
| `deploy/docker-compose.production.yml` | User-facing compose template (attached to GitHub Releases) |
| `internal/server/secrets/jwt.go` | Read-or-generate JWT secret from env or file |
| `internal/server/webui/embed.go` | `//go:embed all:static` for server-side SPA embedding |

### Removed

| File | Reason |
|------|--------|
| `deploy/docker-compose.yml` | Replaced by `docker-compose.production.yml` |
| `deploy/caddy/` | TLS handled by autocert in Go server |
| `deploy/nginx/` | SPA served by Go server via embed |
| `deploy/Dockerfile.builder` | Replaced by CI cross-compilation |
| `deploy/builder-entrypoint.sh` | Replaced by CI cross-compilation |

### Unchanged

| File | Why |
|------|-----|
| `deploy/docker-compose.local.yml` | Local dev stays as-is (no FQDN, plain HTTP :8080) |
| Binary patching system | Same `bytes.Replace` logic, different binary source |
| All client code, client SPA, API routes | No changes |

## User Experience

### Deploying Trasker (end user perspective)

1. Download `docker-compose.production.yml` from the GitHub Releases page
2. Edit: FQDN, admin email, admin password, postgres password
3. `docker compose -f docker-compose.production.yml up -d`
4. Visit `https://<fqdn>` — TLS cert acquired automatically
5. Log in with admin credentials
6. Download client binaries from the admin dashboard (pre-configured with server URL)

### Local development (contributor perspective)

No change. `cd deploy && docker compose -f docker-compose.local.yml up --build` — same as today.
