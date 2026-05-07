# Decisions Intel

No ADRs were ingested in this batch. This file is intentionally near-empty so downstream consumers (`gsd-roadmapper`) get a stable file path.

The architectural and technology choices observed in the SPEC and DOC inputs are recorded as **constraints** in `constraints.md` (since they originate from a design SPEC, not a formal Accepted ADR with `locked: true`). Promote any of them to a locked ADR before treating them as immovable.

## Doc-derived directional choices (NOT locked)

These read like decisions but originate in `docs/superpowers/specs/2026-03-23-trasker-design.md` (Status: Draft) and `docs/superpowers/specs/2026-03-26-production-deployment-design.md`. They have SPEC-level authority only — any future ADR can override them.

- **Self-hosted-per-org deployment model** (no multi-tenant SaaS in v1)
  source: docs/superpowers/specs/2026-03-23-trasker-design.md (Overview, "Deployment Model")
- **Pure-Go SQLite via `modernc.org/sqlite`** (chosen specifically to avoid CGo cross-compilation pain)
  source: docs/superpowers/specs/2026-03-23-trasker-design.md (Tech Stack, Build & Distribution)
- **Single Go module monorepo** (cmd/trasker-client + cmd/trasker-server + internal/{client,server,shared})
  source: docs/superpowers/specs/2026-03-23-trasker-design.md (Repository Structure)
- **Entra ID OIDC for dashboard auth + bcrypt-hashed API keys for client auth**
  source: docs/superpowers/specs/2026-03-23-trasker-design.md (Authentication, API Key Lifecycle)
- **No keystroke / mouse / screenshot monitoring — focus tracking only**
  source: docs/superpowers/specs/2026-03-23-trasker-design.md (Philosophy, Security)
- **Submitted entries are immutable from the client; only org admin can modify via audit-logged endpoints**
  source: docs/superpowers/specs/2026-03-23-trasker-design.md (Data Sovereignty, Submit Flow)
- **Pivot from 4-container (Caddy + Nginx + API + Postgres) to 2-container (trasker + Postgres)**
  source: docs/superpowers/specs/2026-03-26-production-deployment-design.md (Goal, Architecture)
- **Native autocert TLS in the Go server (no external TLS tooling)**
  source: docs/superpowers/specs/2026-03-26-production-deployment-design.md (Autocert TLS Integration)
- **CI cross-compiles all 5 client targets with sentinel placeholders; runtime patches via `bytes.Replace` (no runtime `go build`)**
  source: docs/superpowers/specs/2026-03-26-production-deployment-design.md (Client Binary Distribution)
- **JWT secret auto-generated to `/data/jwt-secret` if env var unset (replaces hard-fail behavior)**
  source: docs/superpowers/specs/2026-03-26-production-deployment-design.md (JWT Secret Auto-Generation)
