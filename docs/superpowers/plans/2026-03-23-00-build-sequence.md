# Trasker — Build Sequence

**Spec:** `docs/superpowers/specs/2026-03-23-trasker-design.md`

## Plan Dependency Order

```
Phase 1 (foundation — must be first):
  Plan 01: Shared Foundation
    Go module init, shared models, API contracts, apikey package, version, Makefile

Phase 2 (parallel — independent of each other, depend on Plan 01):
  Plan 02: Server Core
    Go API server, PostgreSQL store, Entra auth, all API endpoints
  Plan 03: Client Core
    Focus tracker (Linux only first), presence detection, SQLite store, session engine

Phase 3 (parallel — depend on their respective Phase 2 plan):
  Plan 04: Server Dashboard & Deployment
    Svelte SPA, Docker Compose, Caddy, migrations, build pipeline
  Plan 05: Client UI & Features
    Tagger, pomodoro, tray, notifications, local Svelte dashboard, sync/submit

Phase 4 (integration):
  Plan 06: Cross-Platform & Integration
    macOS + Windows focus trackers, cross-compile, end-to-end testing
```

## Parallelization for Agent Swarm

Plans 02 + 03 can run in parallel (different directories, no shared files).
Plans 04 + 05 can run in parallel (server web/ vs client internal/).
Plan 06 is sequential (needs everything working first).
